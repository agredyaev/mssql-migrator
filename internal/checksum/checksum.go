package checksum

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func NormalizeSQL(input string) string {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.ReplaceAll(input, "\r", "\n")
	lines := strings.Split(input, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

func SHA256String(input string) string {
	sum := sha256.Sum256([]byte(NormalizeSQL(input)))
	return hex.EncodeToString(sum[:])
}

func SHA256File(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return SHA256String(string(b)), nil
}

func SQLDirHash(root string) (string, error) {
	entries := []string{}
	for _, sub := range []string{"versioned", "repeatable", "checks"} {
		base := filepath.Join(root, sub)
		if _, err := os.Stat(base); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || filepath.Ext(path) != ".sql" {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			h, err := SHA256File(path)
			if err != nil {
				return err
			}
			entries = append(entries, filepath.ToSlash(rel)+":"+h)
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	sort.Strings(entries)
	sum := sha256.Sum256([]byte(strings.Join(entries, "\n")))
	return hex.EncodeToString(sum[:]), nil
}
