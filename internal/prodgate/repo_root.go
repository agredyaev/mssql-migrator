package prodgate

import (
	"os"
	"path/filepath"
)

// FindRepoRoot walks upward from startDir looking for a .git directory.
func FindRepoRoot(startDir string) (string, bool) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", false
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
