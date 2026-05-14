package fs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createTestLayout(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	mustMkdir := func(parts ...string) string {
		p := filepath.Join(parts...)
		os.MkdirAll(p, 0755)
		return p
	}
	writeFile := func(path, content string) {
		mustMkdir(filepath.Dir(path))
		os.WriteFile(path, []byte(content), 0644)
	}

	mustMkdir(dir, "dactests", "reporting", "views")
	mustMkdir(dir, "dactests", "reporting", "procedures")
	mustMkdir(dir, "dactests", "reporting", "functions")
	mustMkdir(dir, "dactests", "reporting", "tables")
	mustMkdir(dir, "dactests", "reporting", "tables", "_migrations", "snapshot")
	mustMkdir(dir, "dactests", "reporting", "checks")

	writeFile(filepath.Join(dir, "dactests", "reporting", "views", "monthly.sql"), "CREATE OR ALTER VIEW monthly AS SELECT 1 AS n")
	writeFile(filepath.Join(dir, "dactests", "reporting", "procedures", "refresh.sql"), "CREATE OR ALTER PROC refresh AS SELECT 1")
	writeFile(filepath.Join(dir, "dactests", "reporting", "tables", "snapshot.sql"), "CREATE TABLE snapshot (id INT)")
	writeFile(filepath.Join(dir, "dactests", "reporting", "tables", "_migrations", "snapshot", "001_deadbee_add_name.sql"), "ALTER TABLE snapshot ADD name VARCHAR(100)")
	writeFile(filepath.Join(dir, "dactests", "reporting", "checks", "daily.sql"), "EXEC tSQLt.RunAll")

	return dir
}

func TestScanDiscoveresSchemas(t *testing.T) {
	dir := createTestLayout(t)
	layout, err := NewScanner().Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(layout.Schemas) != 1 {
		t.Fatalf("expected 1 schema, got %d", len(layout.Schemas))
	}
	if layout.Schemas[0].Name != "reporting" {
		t.Errorf("schema name = %q", layout.Schemas[0].Name)
	}
}

func TestScanDiscoveresObjects(t *testing.T) {
	dir := createTestLayout(t)
	layout, err := NewScanner().Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(layout.Objects) < 3 {
		t.Fatalf("expected >=3 objects, got %d", len(layout.Objects))
	}
}

func TestScanDiscoveresTransitions(t *testing.T) {
	dir := createTestLayout(t)
	layout, err := NewScanner().Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(layout.Transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(layout.Transitions))
	}
	ts := layout.Transitions[0]
	if ts.Ordinal != "001" {
		t.Errorf("ordinal = %q", ts.Ordinal)
	}
	if ts.Commit != "deadbee" {
		t.Errorf("commit = %q", ts.Commit)
	}
	if ts.Slug != "add_name" {
		t.Errorf("slug = %q", ts.Slug)
	}
	if ts.TableName != "snapshot" {
		t.Errorf("table = %q", ts.TableName)
	}
}

func TestScanDiscoveresChecks(t *testing.T) {
	dir := createTestLayout(t)
	layout, err := NewScanner().Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(layout.Checks) != 1 {
		t.Fatalf("expected 1 check script, got %d", len(layout.Checks))
	}
	if layout.Checks[0].Name != "daily" {
		t.Errorf("check name = %q", layout.Checks[0].Name)
	}
}

func TestScanEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	layout, err := NewScanner().Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(layout.Schemas) != 0 {
		t.Errorf("expected 0 schemas in empty dir")
	}
}

func TestScanInvalidRoot(t *testing.T) {
	_, err := NewScanner().Scan(context.Background(), "/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for invalid root")
	}
}

func TestScanRejectsMalformedTransitionFileName(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "dactests", "reporting", "tables", "_migrations", "snapshot"), 0755)
	os.WriteFile(filepath.Join(dir, "dactests", "reporting", "tables", "_migrations", "snapshot", "bad.sql"), []byte("ALTER TABLE"), 0644)

	layout, err := NewScanner().Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(layout.Transitions) != 0 {
		t.Errorf("malformed transition file should be ignored")
	}
}

func TestLazyContentNotLoadedOnScan(t *testing.T) {
	dir := createTestLayout(t)
	layout, _ := NewScanner().Scan(context.Background(), dir)

	obj := layout.Objects[0]
	content, err := obj.Content()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content == "" {
		t.Fatal("content is empty")
	}
}

func TestLazyChecksum(t *testing.T) {
	dir := createTestLayout(t)
	layout, _ := NewScanner().Scan(context.Background(), dir)

	obj := layout.Objects[0]
	cs1, err := obj.Checksum()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cs2, _ := obj.Checksum()
	if cs1 != cs2 {
		t.Errorf("checksum not stable: %q vs %q", cs1, cs2)
	}
	if cs1 == "" {
		t.Fatal("checksum is empty")
	}
}

func TestTransitionScaffoldDetection(t *testing.T) {
	dir := t.TempDir()
	migrationsDir := filepath.Join(dir, "dactests", "reporting", "tables", "_migrations", "snapshot")
	os.MkdirAll(migrationsDir, 0755)

	os.WriteFile(filepath.Join(migrationsDir, "001_deadbee_add_name.sql"),
		[]byte("-- rmig: transition-scaffold\n-- Replace this scaffold\n"), 0644)

	layout, _ := NewScanner().Scan(context.Background(), dir)
	if len(layout.Transitions) != 1 {
		t.Fatalf("expected 1 transition")
	}
	if !layout.Transitions[0].Scaffold {
		t.Error("expected scaffold flag to be true")
	}
}

func TestTransitionNonScaffold(t *testing.T) {
	dir := t.TempDir()
	migrationsDir := filepath.Join(dir, "dactests", "reporting", "tables", "_migrations", "snapshot")
	os.MkdirAll(migrationsDir, 0755)

	os.WriteFile(filepath.Join(migrationsDir, "001_deadbee_add_name.sql"),
		[]byte("ALTER TABLE snapshot ADD name VARCHAR(100);\n"), 0644)

	layout, _ := NewScanner().Scan(context.Background(), dir)
	if len(layout.Transitions) != 1 {
		t.Fatalf("expected 1 transition")
	}
	if layout.Transitions[0].Scaffold {
		t.Error("expected scaffold flag to be false")
	}
}

func TestNormalizedKeys(t *testing.T) {
	dir := createTestLayout(t)
	layout, _ := NewScanner().Scan(context.Background(), dir)

	keys := layout.NormalizedKeys()
	for _, k := range keys {
		if !strings.Contains(k, "/") {
			t.Errorf("key %q does not contain /", k)
		}
	}
}

func TestTransitionSorting(t *testing.T) {
	dir := t.TempDir()
	migrationsDir := filepath.Join(dir, "dactests", "reporting", "tables", "_migrations", "t1")
	if err := os.MkdirAll(migrationsDir, 0755); err != nil {
		t.Fatal(err)
	}
	createSQLFile(t, dir, "dactests/reporting/tables/_migrations/t1/003_cafebabe_third.sql", "-- rmig: transition")
	createSQLFile(t, dir, "dactests/reporting/tables/_migrations/t1/001_deadbeef_first.sql", "-- rmig: transition")
	createSQLFile(t, dir, "dactests/reporting/tables/_migrations/t1/002_beef1234_second.sql", "-- rmig: transition")

	scanner := NewScanner()
	layout, err := scanner.Scan(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(layout.Transitions) != 3 {
		t.Fatalf("expected 3 transitions, got %d", len(layout.Transitions))
	}
	for i, want := range []string{"001", "002", "003"} {
		if layout.Transitions[i].Ordinal != want {
			t.Errorf("transition[%d] ordinal = %q, want %q", i, layout.Transitions[i].Ordinal, want)
		}
	}
}

func TestHasExecutableTransition(t *testing.T) {
	layout := Layout{
		Transitions: []*TransitionScript{
			{Scaffold: true},
			{Scaffold: true},
		},
	}
	if layout.HasExecutableTransition() {
		t.Error("expected false when all are scaffolds")
	}

	layout.Transitions = append(layout.Transitions, &TransitionScript{Scaffold: false})
	if !layout.HasExecutableTransition() {
		t.Error("expected true when at least one is non-scaffold")
	}

	empty := Layout{}
	if empty.HasExecutableTransition() {
		t.Error("expected false when no transitions")
	}
}

func TestLayoutHash_Deterministic(t *testing.T) {
	dir := t.TempDir()
	createSQLFile(t, dir, "dactests/reporting/views/v1.sql", "CREATE VIEW reporting.v1 AS SELECT 1 AS x")
	createSQLFile(t, dir, "dactests/reporting/views/v2.sql", "CREATE VIEW reporting.v2 AS SELECT 2 AS x")

	scanner := NewScanner()
	layout1, _ := scanner.Scan(context.Background(), dir)
	layout2, _ := scanner.Scan(context.Background(), dir)

	h1, err := layout1.LayoutHash()
	if err != nil {
		t.Fatal(err)
	}
	h2, err := layout2.LayoutHash()
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Errorf("LayoutHash not deterministic: %q != %q", h1, h2)
	}
	if h1 == "" {
		t.Error("LayoutHash should not be empty")
	}
}

func createSQLFile(t *testing.T, base, relPath, content string) {
	t.Helper()
	abs := filepath.Join(base, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
