package fs

import (
	"os"
	"path/filepath"
	"strings"
)

func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

func WriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}

func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func HasNonScaffoldSQL(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".sql" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		if !strings.Contains(string(data), TransitionScaffoldDirective) {
			return true
		}
	}
	return false
}
