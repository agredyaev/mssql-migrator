package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"reporting-db-migrations/internal/checksum"
)

const TransitionScaffoldDirective = "-- rmig: transition-scaffold"

var allowedKinds = map[string]struct{}{
	"tables":     {},
	"views":      {},
	"procedures": {},
	"functions":  {},
	"triggers":   {},
	"indexes":    {},
	"types":      {},
	"sequences":  {},
	"synonyms":   {},
}

var moduleKinds = map[string]struct{}{
	"views":      {},
	"procedures": {},
	"functions":  {},
	"triggers":   {},
}

var sqlIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_@$#]*$`)

type Layout struct {
	RootPath    string
	Schemas     []Schema
	Objects     []Object
	Transitions []TransitionScript
	Checks      []CheckScript
}

type Schema struct {
	Name           string
	NormalizedName string
}

type Object struct {
	Path                 string
	AbsolutePath         string
	Content              string
	SchemaName           string
	NormalizedSchemaName string
	Kind                 string
	ObjectName           string
	NormalizedObjectName string
	ParentName           string
	NormalizedParentName string
	NormalizedKey        string
	Checksum             string
	NoTransaction        bool
	Extension            string
}

type CheckScript struct {
	Path          string
	AbsolutePath  string
	Content       string
	SchemaName    string
	Checksum      string
	Name          string
	NoTransaction bool
}

type TransitionScript struct {
	Path          string
	AbsolutePath  string
	Content       string
	SchemaName    string
	TableName     string
	NormalizedKey string
	Checksum      string
	Ordinal       string
	Commit        string
	Slug          string
	NoTransaction bool
	Scaffold      bool
}

type Batch struct {
	SQL    string
	Repeat int
}

func DiscoverLayout(root string) (Layout, error) {
	return discoverLayout(root, false)
}

func DiscoverValidationLayout(root string) (Layout, error) {
	return discoverLayout(root, true)
}

func discoverLayout(root string, includeChecks bool) (Layout, error) {
	info, err := os.Stat(root)
	if err != nil {
		return Layout{}, err
	}
	if !info.IsDir() {
		return Layout{}, fmt.Errorf("invalid repository layout: %s is not a directory", root)
	}

	schemaSet := map[string]Schema{}
	objects := []Object{}
	checks := []CheckScript{}
	transitions := []TransitionScript{}
	seenKeys := map[string]string{}

	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			if entry.IsDir() && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if path != root {
				if err := validateLayoutDirectory(root, path, schemaSet); err != nil {
					return err
				}
			}
			return nil
		}
		if filepath.Ext(name) != ".sql" {
			return fmt.Errorf("invalid repository layout: non-sql file %s", filepath.ToSlash(path))
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		segments := strings.Split(rel, "/")
		if len(segments) < 3 {
			return fmt.Errorf("invalid repository layout: path %s is too short", rel)
		}

		schemaName := strings.TrimSpace(segments[0])
		if schemaName == "" {
			return fmt.Errorf("invalid repository layout: empty schema segment in %s", rel)
		}
		if err := rememberSchema(schemaSet, schemaName); err != nil {
			return err
		}

		kind := strings.TrimSpace(segments[1])
		if kind == "tables" && len(segments) >= 4 && segments[2] == "_migrations" {
			transition, err := parseTransitionScript(root, rel, schemaName)
			if err != nil {
				return err
			}
			transitions = append(transitions, transition)
			return nil
		}
		if kind == "checks" {
			if len(segments) != 3 {
				return fmt.Errorf("invalid repository layout: checks path must be <schema>/checks/<name>.sql: %s", rel)
			}
			check, err := parseCheckScript(root, rel, schemaName)
			if err != nil {
				return err
			}
			if !includeChecks {
				return nil
			}
			checks = append(checks, check)
			return nil
		}
		if _, ok := allowedKinds[kind]; !ok {
			return fmt.Errorf("invalid repository layout: unsupported kind %s in %s", kind, rel)
		}

		object, err := parseLayoutObject(root, rel, schemaName, kind)
		if err != nil {
			return err
		}
		if previous, exists := seenKeys[object.NormalizedKey]; exists {
			return fmt.Errorf("invalid repository layout: duplicate object key %s for %s and %s", object.NormalizedKey, previous, object.Path)
		}
		seenKeys[object.NormalizedKey] = object.Path
		objects = append(objects, object)
		return nil
	})
	if err != nil {
		return Layout{}, err
	}

	schemas := make([]Schema, 0, len(schemaSet))
	for _, schema := range schemaSet {
		schemas = append(schemas, schema)
	}
	sort.Slice(schemas, func(i, j int) bool {
		return schemas[i].NormalizedName < schemas[j].NormalizedName
	})
	sort.Slice(objects, func(i, j int) bool {
		return objects[i].NormalizedKey < objects[j].NormalizedKey
	})
	sort.Slice(transitions, func(i, j int) bool {
		if transitions[i].NormalizedKey != transitions[j].NormalizedKey {
			return transitions[i].NormalizedKey < transitions[j].NormalizedKey
		}
		if transitions[i].Ordinal != transitions[j].Ordinal {
			return transitions[i].Ordinal < transitions[j].Ordinal
		}
		return transitions[i].Path < transitions[j].Path
	})
	sort.Slice(checks, func(i, j int) bool {
		return checks[i].Path < checks[j].Path
	})

	if len(objects) == 0 {
		return Layout{}, fmt.Errorf("invalid repository layout: no managed SQL objects found under %s", filepath.ToSlash(root))
	}

	return Layout{RootPath: root, Schemas: schemas, Objects: objects, Transitions: transitions, Checks: checks}, nil
}

func validateLayoutDirectory(root string, path string, schemaSet map[string]Schema) error {
	relDir, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	relDir = filepath.ToSlash(relDir)
	segments := strings.Split(relDir, "/")
	if len(segments) == 0 {
		return nil
	}
	schemaName := strings.TrimSpace(segments[0])
	if schemaName == "" {
		return fmt.Errorf("invalid repository layout: empty schema segment in %s", relDir)
	}
	if err := rememberSchema(schemaSet, schemaName); err != nil {
		return err
	}
	if len(segments) == 1 {
		return nil
	}

	kind := strings.TrimSpace(segments[1])
	if kind == "" {
		return fmt.Errorf("invalid repository layout: unsupported kind %s in %s", kind, relDir)
	}
	if kind != "checks" {
		if kind == "tables" && len(segments) >= 3 && segments[2] == "_migrations" {
			switch len(segments) {
			case 3, 4:
				return nil
			case 5:
				return nil
			default:
				return fmt.Errorf("invalid repository layout: table migrations path must be <schema>/tables/_migrations/<table>/<nnn>_<commit>_<slug>.sql: %s", relDir)
			}
		}
		if _, ok := allowedKinds[kind]; !ok {
			return fmt.Errorf("invalid repository layout: unsupported kind %s in %s", kind, relDir)
		}
	}

	switch len(segments) {
	case 2:
		return nil
	case 3:
		switch kind {
		case "indexes", "triggers":
			if strings.TrimSpace(segments[2]) == "" {
				return fmt.Errorf("invalid repository layout: empty parent name in %s", relDir)
			}
			return nil
		case "checks":
			return fmt.Errorf("invalid repository layout: checks path must be <schema>/checks/<name>.sql: %s", relDir)
		default:
			return fmt.Errorf("invalid repository layout: %s path must be <schema>/%s/<name>.sql: %s", kind, kind, relDir)
		}
	default:
		switch kind {
		case "indexes", "triggers":
			return fmt.Errorf("invalid repository layout: %s path must be <schema>/%s/<parent>/<name>.sql: %s", kind, kind, relDir)
		case "checks":
			return fmt.Errorf("invalid repository layout: checks path must be <schema>/checks/<name>.sql: %s", relDir)
		default:
			return fmt.Errorf("invalid repository layout: %s path must be <schema>/%s/<name>.sql: %s", kind, kind, relDir)
		}
	}
}

func parseLayoutObject(root string, rel string, schemaName string, kind string) (Object, error) {
	segments := strings.Split(rel, "/")
	abs := filepath.Join(root, filepath.FromSlash(rel))
	fileChecksum, err := checksum.SHA256File(abs)
	if err != nil {
		return Object{}, err
	}
	body, err := os.ReadFile(abs)
	if err != nil {
		return Object{}, err
	}
	object := Object{
		Path:                 rel,
		AbsolutePath:         abs,
		Content:              string(body),
		SchemaName:           schemaName,
		NormalizedSchemaName: strings.ToLower(schemaName),
		Kind:                 kind,
		Checksum:             fileChecksum,
		NoTransaction:        hasNoTransactionDirective(string(body)),
		Extension:            filepath.Ext(segments[len(segments)-1]),
	}
	if object.Extension != ".sql" {
		return Object{}, fmt.Errorf("invalid repository layout: expected .sql object file: %s", rel)
	}

	switch kind {
	case "indexes", "triggers":
		if len(segments) != 4 {
			return Object{}, fmt.Errorf("invalid repository layout: %s path must be <schema>/%s/<parent>/<name>.sql: %s", kind, kind, rel)
		}
		object.ParentName = segments[2]
		if err := validateSQLIdentifier("parent", object.ParentName); err != nil {
			return Object{}, err
		}
		object.NormalizedParentName = strings.ToLower(object.ParentName)
		object.ObjectName = strings.TrimSuffix(segments[3], filepath.Ext(segments[3]))
	default:
		if len(segments) != 3 {
			return Object{}, fmt.Errorf("invalid repository layout: %s path must be <schema>/%s/<name>.sql: %s", kind, kind, rel)
		}
		object.ObjectName = strings.TrimSuffix(segments[2], filepath.Ext(segments[2]))
	}
	if strings.TrimSpace(object.ObjectName) == "" {
		return Object{}, fmt.Errorf("invalid repository layout: empty object name in %s", rel)
	}
	if err := validateSQLIdentifier("object", object.ObjectName); err != nil {
		return Object{}, err
	}
	if kind == "indexes" || kind == "triggers" {
		if strings.TrimSpace(object.ParentName) == "" {
			return Object{}, fmt.Errorf("invalid repository layout: empty parent name in %s", rel)
		}
	}
	object.NormalizedObjectName = strings.ToLower(object.ObjectName)
	object.NormalizedKey = buildNormalizedKey(object)
	return object, nil
}

func parseCheckScript(root string, rel string, schemaName string) (CheckScript, error) {
	abs := filepath.Join(root, filepath.FromSlash(rel))
	fileChecksum, err := checksum.SHA256File(abs)
	if err != nil {
		return CheckScript{}, err
	}
	body, err := os.ReadFile(abs)
	if err != nil {
		return CheckScript{}, err
	}
	name := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	if strings.TrimSpace(name) == "" {
		return CheckScript{}, fmt.Errorf("invalid repository layout: empty check script name in %s", rel)
	}
	return CheckScript{
		Path:          rel,
		AbsolutePath:  abs,
		Content:       string(body),
		SchemaName:    schemaName,
		Checksum:      fileChecksum,
		Name:          name,
		NoTransaction: hasNoTransactionDirective(string(body)),
	}, nil
}

var transitionFilePattern = regexp.MustCompile(`^(\d{3})_([A-Fa-f0-9]{7,40})_([A-Za-z0-9_]+)\.sql$`)
var gitCommitPattern = regexp.MustCompile(`^[A-Fa-f0-9]{7,40}$`)

func parseTransitionScript(root string, rel string, schemaName string) (TransitionScript, error) {
	segments := strings.Split(rel, "/")
	if len(segments) != 5 || segments[1] != "tables" || segments[2] != "_migrations" {
		return TransitionScript{}, fmt.Errorf("invalid repository layout: table migrations path must be <schema>/tables/_migrations/<table>/<nnn>_<commit>_<slug>.sql: %s", rel)
	}
	tableName := strings.TrimSpace(segments[3])
	if err := validateSQLIdentifier("object", tableName); err != nil {
		return TransitionScript{}, err
	}
	fileName := segments[4]
	matches := transitionFilePattern.FindStringSubmatch(fileName)
	if len(matches) != 4 {
		return TransitionScript{}, fmt.Errorf("invalid repository layout: table migration file must match <nnn>_<commit>_<slug>.sql: %s", rel)
	}
	abs := filepath.Join(root, filepath.FromSlash(rel))
	fileChecksum, err := checksum.SHA256File(abs)
	if err != nil {
		return TransitionScript{}, err
	}
	body, err := os.ReadFile(abs)
	if err != nil {
		return TransitionScript{}, err
	}
	return TransitionScript{
		Path:          rel,
		AbsolutePath:  abs,
		Content:       string(body),
		SchemaName:    schemaName,
		TableName:     tableName,
		NormalizedKey: strings.ToLower(strings.TrimSpace(schemaName)) + "/tables/" + strings.ToLower(tableName),
		Checksum:      fileChecksum,
		Ordinal:       matches[1],
		Commit:        strings.ToLower(matches[2]),
		Slug:          matches[3],
		NoTransaction: hasNoTransactionDirective(string(body)),
		Scaffold:      hasTransitionScaffoldDirective(string(body)),
	}, nil
}

func buildNormalizedKey(object Object) string {
	parts := []string{object.NormalizedSchemaName, object.Kind}
	if object.NormalizedParentName != "" {
		parts = append(parts, object.NormalizedParentName)
	}
	parts = append(parts, object.NormalizedObjectName)
	return strings.Join(parts, "/")
}

func IsModuleKind(kind string) bool {
	_, ok := moduleKinds[kind]
	return ok
}

func SupportsExistingObjectUpdate(object Object) bool {
	if !IsModuleKind(object.Kind) {
		return false
	}
	trimmed := strings.TrimSpace(strings.TrimPrefix(object.Content, "\ufeff"))
	upper := strings.ToUpper(trimmed)
	switch object.Kind {
	case "views":
		return strings.HasPrefix(upper, "CREATE OR ALTER VIEW")
	case "procedures":
		return strings.HasPrefix(upper, "CREATE OR ALTER PROCEDURE") || strings.HasPrefix(upper, "CREATE OR ALTER PROC")
	case "functions":
		return strings.HasPrefix(upper, "CREATE OR ALTER FUNCTION")
	case "triggers":
		return strings.HasPrefix(upper, "CREATE OR ALTER TRIGGER")
	default:
		return false
	}
}

func NormalizeTrackedName(value string) string {
	value = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, `\`, "/")))
	if value == "" {
		return ""
	}
	segments := strings.Split(value, "/")
	if len(segments) < 3 || filepath.Ext(segments[len(segments)-1]) != ".sql" {
		return value
	}
	if segments[1] == "checks" {
		segments[len(segments)-1] = strings.TrimSuffix(segments[len(segments)-1], ".sql")
		return strings.Join(segments, "/")
	}
	if _, ok := allowedKinds[segments[1]]; !ok {
		return value
	}
	segments[len(segments)-1] = strings.TrimSuffix(segments[len(segments)-1], ".sql")
	if (segments[1] == "indexes" || segments[1] == "triggers") && len(segments) == 4 {
		return strings.Join(segments, "/")
	}
	if len(segments) == 3 {
		return strings.Join(segments, "/")
	}
	return value
}

func rememberSchema(schemaSet map[string]Schema, schemaName string) error {
	normalized := strings.ToLower(strings.TrimSpace(schemaName))
	if normalized == "" {
		return nil
	}
	if err := validateSQLIdentifier("schema", schemaName); err != nil {
		return err
	}
	if existing, ok := schemaSet[normalized]; ok && existing.Name != schemaName {
		return fmt.Errorf("invalid repository layout: schema casing conflict for %s and %s", existing.Name, schemaName)
	}
	schemaSet[normalized] = Schema{Name: schemaName, NormalizedName: normalized}
	return nil
}

func validateSQLIdentifier(kind string, value string) error {
	value = strings.TrimSpace(value)
	if !sqlIdentifierPattern.MatchString(value) {
		return fmt.Errorf("invalid repository layout: invalid %s identifier %q", kind, value)
	}
	return nil
}

func HashLayout(layout Layout, includeChecks bool) string {
	entries := make([]string, 0, len(layout.Schemas)+len(layout.Objects)+len(layout.Transitions)+len(layout.Checks))
	for _, schema := range layout.Schemas {
		entries = append(entries, "schema:"+schema.NormalizedName)
	}
	for _, object := range layout.Objects {
		entries = append(entries, "object:"+object.Path+":"+object.NormalizedKey+":"+object.Checksum)
	}
	for _, transition := range layout.Transitions {
		entries = append(entries, "transition:"+transition.Path+":"+transition.NormalizedKey+":"+transition.Checksum)
	}
	if includeChecks {
		for _, check := range layout.Checks {
			entries = append(entries, "check:"+check.Path+":"+check.Checksum)
		}
	}
	sort.Strings(entries)
	sum := sha256.Sum256([]byte(strings.Join(entries, "\n")))
	return hex.EncodeToString(sum[:])
}

func TransitionCommitToken(gitCommit string) string {
	normalized := strings.ToLower(strings.TrimSpace(gitCommit))
	if normalized == "" {
		return ""
	}
	if gitCommitPattern.MatchString(normalized) {
		if len(normalized) <= 7 {
			return normalized
		}
		return normalized[:7]
	}
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])[:7]
}

func HasExecutableTransition(items []TransitionScript) bool {
	for _, item := range items {
		if !item.Scaffold {
			return true
		}
	}
	return false
}

func hasNoTransactionDirective(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		if trimmed == "-- migrator: no-transaction" {
			return true
		}
	}
	return false
}

func hasTransitionScaffoldDirective(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		if trimmed == TransitionScaffoldDirective {
			return true
		}
	}
	return false
}
