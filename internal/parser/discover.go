package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"reporting-db-migrations/internal/checksum"
)

var versionedPattern = regexp.MustCompile(`^V([0-9]+)__([A-Za-z0-9_\-]+)\.sql$`)
var repeatablePattern = regexp.MustCompile(`^R([0-9]+)__([A-Za-z0-9_\-]+)\.sql$`)
var checkPattern = regexp.MustCompile(`^C([0-9]+)__([A-Za-z0-9_\-]+)\.sql$`)

func Discover(root string) ([]Script, []Script, []Script, error) {
	versioned, err := discoverType(filepath.Join(root, "versioned"), ScriptTypeVersioned)
	if err != nil {
		return nil, nil, nil, err
	}
	repeatable, err := discoverType(filepath.Join(root, "repeatable"), ScriptTypeRepeatable)
	if err != nil {
		return nil, nil, nil, err
	}
	checks, err := discoverType(filepath.Join(root, "checks"), ScriptTypeCheck)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := rejectDuplicateVersions(versioned); err != nil {
		return nil, nil, nil, err
	}
	return versioned, repeatable, checks, nil
}

func discoverType(directory string, scriptType ScriptType) ([]Script, error) {
	if _, err := os.Stat(directory); os.IsNotExist(err) {
		return []Script{}, nil
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	scripts := make([]Script, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		script, err := ParseScript(filepath.Join(directory, entry.Name()), scriptType)
		if err != nil {
			return nil, err
		}
		scripts = append(scripts, script)
	}
	sort.Slice(scripts, func(i int, j int) bool { return scripts[i].Name < scripts[j].Name })
	return scripts, nil
}

func ParseScript(path string, scriptType ScriptType) (Script, error) {
	name := filepath.Base(path)
	matches := matchName(name, scriptType)
	if len(matches) != 3 {
		return Script{}, fmt.Errorf("invalid %s migration filename: %s", scriptType, name)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return Script{}, err
	}
	fileChecksum, err := checksum.SHA256File(path)
	if err != nil {
		return Script{}, err
	}
	body := string(content)
	return Script{Name: name, Path: path, Type: scriptType, Version: matches[1], Description: matches[2], Checksum: fileChecksum, NoTransaction: strings.Contains(body, "-- migrator: no-transaction"), RequiresApproval: strings.Contains(body, "-- migrator: requires-approval")}, nil
}

func matchName(name string, scriptType ScriptType) []string {
	switch scriptType {
	case ScriptTypeVersioned:
		return versionedPattern.FindStringSubmatch(name)
	case ScriptTypeRepeatable:
		return repeatablePattern.FindStringSubmatch(name)
	case ScriptTypeCheck:
		return checkPattern.FindStringSubmatch(name)
	default:
		return nil
	}
}

func rejectDuplicateVersions(scripts []Script) error {
	seen := map[string]string{}
	for _, script := range scripts {
		previousName, exists := seen[script.Version]
		if exists {
			return fmt.Errorf("duplicate version V%s: %s and %s", script.Version, previousName, script.Name)
		}
		seen[script.Version] = script.Name
	}
	return nil
}
