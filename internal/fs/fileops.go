package fs

import (
	"bufio"
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
		f, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		firstLine, _ := bufio.NewReader(f).ReadString('\n')
		f.Close()
		if !strings.HasPrefix(strings.TrimRight(firstLine, "\r\n"), TransitionScaffoldDirective) {
			return true
		}
	}
	return false
}
