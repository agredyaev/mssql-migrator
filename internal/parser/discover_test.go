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
}

func TestDiscoverSortsVersionsNumerically(t *testing.T) {
	root := t.TempDir()
	versionedDir := filepath.Join(root, "versioned")
	if err := os.MkdirAll(versionedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(versionedDir, "V002__two.sql"), "SELECT 2;")
	writeFile(t, filepath.Join(versionedDir, "V010__ten.sql"), "SELECT 10;")
	versioned, _, _, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(versioned) != 2 {
		t.Fatalf("expected 2 scripts, got %d", len(versioned))
	}
	if versioned[0].Name != "V002__two.sql" || versioned[1].Name != "V010__ten.sql" {
		t.Fatalf("unexpected sort order: %#v", versioned)
	}
}

func TestDiscoverRejectsNonPaddedVersionNames(t *testing.T) {
	root := t.TempDir()
	versionedDir := filepath.Join(root, "versioned")
	if err := os.MkdirAll(versionedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(versionedDir, "V2__two.sql"), "SELECT 2;")
	_, _, _, err := Discover(root)
	if err == nil {
		t.Fatal("expected invalid filename error")
	}
}

func TestParseScriptVersion(t *testing.T) {
	version, err := ParseScriptVersion("V010")
	if err != nil {
		t.Fatal(err)
	}
	if version != "010" {
		t.Fatalf("unexpected version: %s", version)
	}
	if _, err := ParseScriptVersion("010"); err == nil {
		t.Fatal("expected invalid version error")
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
