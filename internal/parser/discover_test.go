package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverRejectsDuplicateVersion(t *testing.T) {
	root := t.TempDir()
	versionedDir := filepath.Join(root, "versioned")
	if err := os.MkdirAll(versionedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(versionedDir, "V001__one.sql"), "SELECT 1;")
	writeFile(t, filepath.Join(versionedDir, "V001__two.sql"), "SELECT 2;")
	_, _, _, err := Discover(root)
	if err == nil {
		t.Fatal("expected duplicate version error")
	}
}

func TestParseScriptDetectsMarkers(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "R001__views.sql")
	writeFile(t, path, "-- migrator: no-transaction\n-- migrator: requires-approval\nSELECT 1;")
	script, err := ParseScript(path, ScriptTypeRepeatable)
	if err != nil {
		t.Fatal(err)
	}
	if !script.NoTransaction {
		t.Fatal("expected no-transaction marker")
	}
	if !script.RequiresApproval {
		t.Fatal("expected requires-approval marker")
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
