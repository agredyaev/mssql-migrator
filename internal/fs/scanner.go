package fs

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"reporting-db-migrations/internal/types"
)

const TransitionScaffoldDirective = "-- rmig: transition-scaffold"

type Scanner struct{}

func NewScanner() *Scanner {
	return &Scanner{}
}

func (s *Scanner) Scan(ctx context.Context, root string) (Layout, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return Layout{}, err
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return Layout{}, fmt.Errorf("invalid or missing SQL scripts root: %s", root)
	}

	layout := Layout{RootPath: root}
	dbDirs, err := os.ReadDir(root)
	if err != nil {
		return Layout{}, err
	}

	for _, dbEntry := range dbDirs {
		if !dbEntry.IsDir() {
			continue
		}
		dbName := dbEntry.Name()
		dbPath := filepath.Join(root, dbName)

		schemaDirs, err := os.ReadDir(dbPath)
		if err != nil {
			return Layout{}, fmt.Errorf("scan: read schema dir %s: %w", dbPath, err)
		}

		for _, schemaEntry := range schemaDirs {
			if !schemaEntry.IsDir() {
				continue
			}
			schemaName := schemaEntry.Name()
			schemaPath := filepath.Join(dbPath, schemaName)

			layout.Schemas = append(layout.Schemas, Schema{
				DatabaseName:   dbName,
				Name:           schemaName,
				NormalizedName: strings.ToLower(schemaName),
			})

			kindDirs, err := os.ReadDir(schemaPath)
			if err != nil {
				return Layout{}, fmt.Errorf("scan: read kind dir %s: %w", schemaPath, err)
			}

			for _, kindEntry := range kindDirs {
				if !kindEntry.IsDir() {
					continue
				}
				kind := kindEntry.Name()
				kindPath := filepath.Join(schemaPath, kind)

				switch {
				case kind == "tables":
					s.scanTableDir(&layout, dbName, schemaName, kindPath)
				case kind == "checks":
					s.scanCheckDir(&layout, dbName, schemaName, kindPath)
				case isObjectKind(kind):
					s.scanObjectDir(&layout, dbName, schemaName, kind, kindPath)
				}
			}
		}
	}

	sort.Slice(layout.Transitions, func(i, j int) bool {
		return layout.Transitions[i].Ordinal < layout.Transitions[j].Ordinal
	})

	return layout, nil
}

func (s *Scanner) scanObjectDir(layout *Layout, dbName, schemaName, kind, dirPath string) {
	files, err := os.ReadDir(dirPath)
	if err != nil {
		return
	}
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".sql" {
			continue
		}
		obj := s.buildObject(dbName, schemaName, kind, "", f.Name(), dirPath)
		layout.Objects = append(layout.Objects, &obj)
	}
}

func (s *Scanner) scanTableDir(layout *Layout, dbName, schemaName, tableDir string) {
	files, err := os.ReadDir(tableDir)
	if err != nil {
		return
	}
	for _, f := range files {
		name := f.Name()
		if f.IsDir() || filepath.Ext(name) != ".sql" {
			continue
		}
		obj := s.buildObject(dbName, schemaName, "tables", "", name, tableDir)
		layout.Objects = append(layout.Objects, &obj)
	}

	migrationsDir := filepath.Join(tableDir, "_migrations")
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return
	}
	for _, tableEntry := range entries {
		if !tableEntry.IsDir() {
			continue
		}
		tableName := tableEntry.Name()
		tableMigrationsDir := filepath.Join(migrationsDir, tableName)

		migrationFiles, err := os.ReadDir(tableMigrationsDir)
		if err != nil {
			continue
		}
		for _, mf := range migrationFiles {
			if mf.IsDir() || filepath.Ext(mf.Name()) != ".sql" {
				continue
			}
			ts, ok := s.parseTransitionFile(dbName, schemaName, tableName, mf.Name(), tableMigrationsDir)
			if ok {
				layout.Transitions = append(layout.Transitions, ts)
			}
		}
	}
}

func (s *Scanner) scanCheckDir(layout *Layout, dbName, schemaName, dirPath string) {
	files, err := os.ReadDir(dirPath)
	if err != nil {
		return
	}
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".sql" {
			continue
		}
		cs := s.buildCheck(dbName, schemaName, f.Name(), dirPath)
		layout.Checks = append(layout.Checks, &cs)
	}
}

func (s *Scanner) buildObject(dbName, schemaName, kind, parentName, fileName, dirPath string) Object {
	fullPath := filepath.Join(dirPath, fileName)
	name := strings.TrimSuffix(fileName, ".sql")
	return Object{
		Path:                 filepath.ToSlash(filepath.Join(dbName, schemaName, kind, fileName)),
		AbsolutePath:         fullPath,
		DatabaseName:         dbName,
		SchemaName:           schemaName,
		NormalizedSchemaName: strings.ToLower(schemaName),
		Kind:                 kind,
		ObjectName:           name,
		ParentName:           parentName,
		NormalizedKey:        types.NormalizedKey(schemaName, kind, name),
		NoTransaction:        types.IsNoTransactionKind(kind),
	}
}

func (s *Scanner) buildCheck(dbName, schemaName, fileName, dirPath string) CheckScript {
	fullPath := filepath.Join(dirPath, fileName)
	name := strings.TrimSuffix(fileName, ".sql")
	return CheckScript{
		Path:          filepath.ToSlash(filepath.Join(dbName, schemaName, "checks", fileName)),
		AbsolutePath:  fullPath,
		DatabaseName:  dbName,
		SchemaName:    schemaName,
		Name:          name,
		NoTransaction: true,
	}
}

var transitionPattern = regexp.MustCompile(`^(\d{3})_([0-9a-f]{7,})_(.+)\.sql$`)

func (s *Scanner) parseTransitionFile(dbName, schemaName, tableName, fileName, dirPath string) (*TransitionScript, bool) {
	matches := transitionPattern.FindStringSubmatch(fileName)
	if matches == nil {
		return nil, false
	}

	fullPath := filepath.Join(dirPath, fileName)
	normalizedKey := types.NormalizedKey(schemaName, "tables", tableName)

	ts := &TransitionScript{
		Path:          filepath.ToSlash(filepath.Join(dbName, schemaName, "tables", "_migrations", tableName, fileName)),
		AbsolutePath:  fullPath,
		DatabaseName:  dbName,
		SchemaName:    schemaName,
		TableName:     tableName,
		NormalizedKey: normalizedKey,
		Ordinal:       matches[1],
		Commit:        matches[2],
		Slug:          matches[3],
	}

	f, err := os.Open(fullPath)
	if err == nil {
		firstLine, _ := bufio.NewReader(f).ReadString('\n')
		f.Close()
		if strings.HasPrefix(firstLine, TransitionScaffoldDirective) {
			ts.Scaffold = true
		}
	}

	return ts, true
}

var allowedKinds = map[string]struct{}{
	"tables": {}, "views": {}, "procedures": {}, "functions": {},
	"triggers": {}, "indexes": {}, "types": {}, "sequences": {}, "synonyms": {},
}

func isObjectKind(kind string) bool {
	_, ok := allowedKinds[kind]
	return ok && kind != "tables" && kind != "checks"
}
