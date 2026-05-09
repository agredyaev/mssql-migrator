package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverLayoutIgnoresHiddenFile(t *testing.T) {
	root := t.TempDir()
	writeLayoutFile(t, filepath.Join(root, "reporting", "views", "monthly.sql"), "SELECT 1;")
	writeLayoutFile(t, filepath.Join(root, "reporting", ".bad.sql"), "SELECT 1;")
	if _, err := DiscoverLayout(root); err != nil {
		t.Fatalf("unexpected hidden file handling error: %v", err)
	}
}

func TestDiscoverLayoutRejectsNonSQLFile(t *testing.T) {
	root := t.TempDir()
	writeLayoutFile(t, filepath.Join(root, "reporting", "views", "monthly.txt"), "bad")
	if _, err := DiscoverLayout(root); err == nil {
		t.Fatal("expected non-sql file error")
	}
}

func TestDiscoverLayoutRejectsDuplicateNormalizedKey(t *testing.T) {
	root := t.TempDir()
	writeLayoutFile(t, filepath.Join(root, "reporting", "views", "Monthly.sql"), "SELECT 1;")
	writeLayoutFile(t, filepath.Join(root, "reporting", "views", "monthly.sql"), "SELECT 2;")
	if _, err := DiscoverLayout(root); err == nil {
		t.Fatal("expected duplicate normalized key error")
	}
}

func TestDiscoverLayoutRejectsSchemaCasingConflict(t *testing.T) {
	root := t.TempDir()
	writeLayoutFile(t, filepath.Join(root, "Reporting", "views", "monthly.sql"), "SELECT 1;")
	writeLayoutFile(t, filepath.Join(root, "reporting", "procedures", "refresh.sql"), "SELECT 2;")
	if _, err := DiscoverLayout(root); err == nil {
		t.Fatal("expected schema casing conflict error")
	}
}

func TestDiscoverValidationLayoutParsesChecksAndNoTransaction(t *testing.T) {
	root := t.TempDir()
	writeLayoutFile(t, filepath.Join(root, "reporting", "views", "monthly.sql"), "SELECT 1;")
	writeLayoutFile(t, filepath.Join(root, "reporting", "checks", "smoke.sql"), "-- migrator: no-transaction\nSELECT 1;")
	layout, err := DiscoverValidationLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(layout.Schemas) != 1 || layout.Schemas[0].NormalizedName != "reporting" {
		t.Fatalf("unexpected schema discovery: %#v", layout.Schemas)
	}
	if len(layout.Checks) != 1 || !layout.Checks[0].NoTransaction {
		t.Fatalf("unexpected checks layout: %#v", layout.Checks)
	}
}

func TestDiscoverLayoutIgnoresChecksInPlanLayout(t *testing.T) {
	root := t.TempDir()
	writeLayoutFile(t, filepath.Join(root, "reporting", "views", "monthly.sql"), "SELECT 1;")
	writeLayoutFile(t, filepath.Join(root, "reporting", "checks", "smoke.sql"), "SELECT 1;")
	layout, err := DiscoverLayout(root)
	if err != nil {
		t.Fatalf("unexpected checks layout error: %v", err)
	}
	if len(layout.Checks) != 0 {
		t.Fatalf("expected plan layout to ignore checks, got %#v", layout.Checks)
	}
}

func TestDiscoverLayoutParsesIndexParentPath(t *testing.T) {
	root := t.TempDir()
	writeLayoutFile(t, filepath.Join(root, "reporting", "indexes", "snapshot", "ix_snapshot.sql"), "CREATE INDEX ix_snapshot ON reporting.snapshot(id);")
	layout, err := DiscoverLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(layout.Objects) != 1 {
		t.Fatalf("expected one object, got %d", len(layout.Objects))
	}
	if layout.Objects[0].ParentName != "snapshot" {
		t.Fatalf("unexpected parent name: %#v", layout.Objects[0])
	}
}

func TestDiscoverLayoutRejectsUnsupportedKind(t *testing.T) {
	root := t.TempDir()
	writeLayoutFile(t, filepath.Join(root, "reporting", "packages", "bad.sql"), "SELECT 1;")
	if _, err := DiscoverLayout(root); err == nil {
		t.Fatal("expected unsupported kind error")
	}
}

func TestDiscoverLayoutRejectsTooShortPath(t *testing.T) {
	root := t.TempDir()
	writeLayoutFile(t, filepath.Join(root, "reporting", "bad.sql"), "SELECT 1;")
	if _, err := DiscoverLayout(root); err == nil {
		t.Fatal("expected too-short path error")
	}
}

func TestDiscoverLayoutRejectsInvalidTriggerDepth(t *testing.T) {
	root := t.TempDir()
	writeLayoutFile(t, filepath.Join(root, "reporting", "triggers", "trg_audit.sql"), "SELECT 1;")
	if _, err := DiscoverLayout(root); err == nil {
		t.Fatal("expected trigger depth error")
	}
}

func TestDiscoverLayoutRejectsEmptyManagedObjectSet(t *testing.T) {
	root := t.TempDir()
	writeLayoutDir(t, filepath.Join(root, "reporting", "views"))
	if _, err := DiscoverLayout(root); err == nil || !strings.Contains(err.Error(), "no managed SQL objects found") {
		t.Fatalf("expected empty managed object set error, got %v", err)
	}
}

func TestDiscoverLayoutRejectsInvalidNestedDirectoryShape(t *testing.T) {
	root := t.TempDir()
	writeLayoutDir(t, filepath.Join(root, "reporting", "views", "monthly"))
	if _, err := DiscoverLayout(root); err == nil || !strings.Contains(err.Error(), "views path must be <schema>/views/<name>.sql") {
		t.Fatalf("expected invalid nested directory error, got %v", err)
	}
}

func TestDiscoverLayoutRequiresExactNoTransactionDirective(t *testing.T) {
	root := t.TempDir()
	writeLayoutFile(t, filepath.Join(root, "reporting", "views", "monthly.sql"), "SELECT '-- migrator: no-transaction';")
	layout, err := DiscoverLayout(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(layout.Objects) != 1 {
		t.Fatalf("expected one object, got %d", len(layout.Objects))
	}
	if layout.Objects[0].NoTransaction {
		t.Fatalf("expected directive parser not to match SQL string literal: %#v", layout.Objects[0])
	}
}

func TestNormalizeTrackedNameRemovesSQLSuffixForRepoObjects(t *testing.T) {
	if got := NormalizeTrackedName(`reporting\views\monthly.sql`); got != "reporting/views/monthly" {
		t.Fatalf("unexpected normalized tracked name: %q", got)
	}
}

func writeLayoutFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeLayoutDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
