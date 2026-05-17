package scaffold

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reporting-db-migrations/internal/db"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/types"
)

func TestEnsureTransitionFiles_NoBlockedTables(t *testing.T) {
	s := New()
	layout := fs.Layout{
		Objects: []*fs.Object{{
			NormalizedKey: "r/views/v1",
			Kind:          "views",
		}},
	}
	plan := &types.MigrationPlan{
		Objects: []types.PlannedObject{{
			ObjectRef:     types.ObjectRef{NormalizedKey: "r/views/v1"},
			PlannedAction: types.ActionCreateObject,
		}},
	}
	columns := map[string][]db.TableColumn{}

	created, err := s.EnsureTransitionFiles(context.Background(), types.Config{}, layout, plan, columns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created {
		t.Error("expected created=false for non-blocked plan")
	}
}

func TestEnsureTransitionFiles_BlockedTableCreatesScaffold(t *testing.T) {
	s := New()
	baseDir := t.TempDir()
	layout := fs.Layout{
		Objects: []*fs.Object{{
			NormalizedKey: "r/tables/t1",
			Kind:          "tables",
			ObjectName:    "t1",
			SchemaName:    "r",
		}},
	}
	plan := &types.MigrationPlan{
		Blocked: true,
		Objects: []types.PlannedObject{{
			ObjectRef:     types.ObjectRef{NormalizedKey: "r/tables/t1", Kind: "tables", ObjectName: "t1", SchemaName: "r"},
			PlannedAction: types.ActionReprocessChangedBlocked,
		}},
	}
	columns := map[string][]db.TableColumn{
		"r/tables/t1": {
			{Name: "id", TypeName: "int", Nullable: false},
			{Name: "name", TypeName: "nvarchar", Nullable: false},
		},
	}
	cfg := types.Config{SQLBase: baseDir}

	created, err := s.EnsureTransitionFiles(context.Background(), cfg, layout, plan, columns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Fatal("expected created=true for blocked table")
	}

	dir := filepath.Join(baseDir, "r", "tables", "_migrations", "t1")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	if len(files) == 0 {
		t.Fatal("no scaffold file created")
	}

	fullPath := filepath.Join(dir, files[0])
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("failed to read generated file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "-- rmig: transition-scaffold") {
		t.Error("scaffold content missing transition-scaffold marker")
	}
	if !strings.Contains(content, "[r].[t1]") {
		t.Error("scaffold content missing table reference")
	}
}

func TestEnsureTransitionFiles_Idempotent(t *testing.T) {
	s := New()
	baseDir := t.TempDir()
	layout := fs.Layout{
		Objects: []*fs.Object{{
			NormalizedKey: "r/tables/t1",
			Kind:          "tables",
			ObjectName:    "t1",
			SchemaName:    "r",
		}},
	}
	plan := &types.MigrationPlan{
		Blocked: true,
		Objects: []types.PlannedObject{{
			ObjectRef:     types.ObjectRef{NormalizedKey: "r/tables/t1", Kind: "tables", ObjectName: "t1", SchemaName: "r"},
			PlannedAction: types.ActionReprocessChangedBlocked,
		}},
	}
	columns := map[string][]db.TableColumn{}
	cfg := types.Config{SQLBase: baseDir}

	created1, err := s.EnsureTransitionFiles(context.Background(), cfg, layout, plan, columns)
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}
	created2, err := s.EnsureTransitionFiles(context.Background(), cfg, layout, plan, columns)
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if !created1 {
		t.Fatal("first call: expected created=true")
	}
	if created2 {
		t.Error("second call: expected created=false (idempotent)")
	}
}

func TestEnsureTransitionFiles_ExistingRealTransitionSkips(t *testing.T) {
	s := New()
	baseDir := t.TempDir()

	targetDir := filepath.Join(baseDir, "r", "tables", "_migrations", "t1")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	os.WriteFile(filepath.Join(targetDir, "001_abc_add_col.sql"), []byte("ALTER TABLE t1 ADD col INT;\n"), 0644)

	layout := fs.Layout{
		Objects: []*fs.Object{{
			NormalizedKey: "r/tables/t1",
			Kind:          "tables",
			ObjectName:    "t1",
			SchemaName:    "r",
		}},
	}
	plan := &types.MigrationPlan{
		Blocked: true,
		Objects: []types.PlannedObject{{
			ObjectRef:     types.ObjectRef{NormalizedKey: "r/tables/t1", Kind: "tables", ObjectName: "t1", SchemaName: "r"},
			PlannedAction: types.ActionReprocessChangedBlocked,
		}},
	}
	columns := map[string][]db.TableColumn{}
	cfg := types.Config{SQLBase: baseDir}

	created, err := s.EnsureTransitionFiles(context.Background(), cfg, layout, plan, columns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created {
		t.Error("expected created=false (real transition exists)")
	}
}

func TestEnsureTransitionFiles_AutoAddColumn(t *testing.T) {
	s := New()
	baseDir := t.TempDir()

	sqlPath := filepath.Join(baseDir, "r", "tables", "t1.sql")
	if err := os.MkdirAll(filepath.Dir(sqlPath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	createTableSQL := "CREATE TABLE [r].[t1] (\n  [id] INT NOT NULL,\n  [new_col] NVARCHAR(100) NULL\n)"
	if err := os.WriteFile(sqlPath, []byte(createTableSQL), 0644); err != nil {
		t.Fatalf("writefile: %v", err)
	}

	layout := fs.Layout{
		Objects: []*fs.Object{{
			CachedFile:    fs.CachedFile{AbsPath: sqlPath},
			Path:          "r/tables/t1.sql",
			NormalizedKey: "r/tables/t1",
			Kind:          "tables",
			ObjectName:    "t1",
			SchemaName:    "r",
		}},
	}
	plan := &types.MigrationPlan{
		Blocked: true,
		Objects: []types.PlannedObject{{
			ObjectRef:     types.ObjectRef{NormalizedKey: "r/tables/t1", Kind: "tables", ObjectName: "t1", SchemaName: "r"},
			PlannedAction: types.ActionReprocessChangedBlocked,
		}},
	}

	dbColumns := map[string][]db.TableColumn{
		"r/tables/t1": {
			{Name: "id", TypeName: "INT", Nullable: false},
		},
	}

	cfg := types.Config{SQLBase: baseDir}
	created, err := s.EnsureTransitionFiles(context.Background(), cfg, layout, plan, dbColumns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Fatal("expected created=true for auto ADD COLUMN")
	}

	dir := filepath.Join(baseDir, "r", "tables", "_migrations", "t1")
	entries, _ := os.ReadDir(dir)
	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	if len(files) == 0 {
		t.Fatal("no migration file created")
	}

	fullPath := filepath.Join(dir, files[0])
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("readfile: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "-- rmig: transition-scaffold") {
		t.Error("auto migration should NOT be a scaffold")
	}
	if !strings.Contains(content, "ALTER TABLE") {
		t.Error("auto migration should contain ALTER TABLE")
	}
	if !strings.Contains(content, "new_col") {
		t.Error("auto migration should reference new column")
	}
}

func TestEnsureTransitionFiles_AutoAddColumn_FallsBackToScaffoldOnDrop(t *testing.T) {
	s := New()
	baseDir := t.TempDir()

	sqlPath := filepath.Join(baseDir, "r", "tables", "t1.sql")
	if err := os.MkdirAll(filepath.Dir(sqlPath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	createTableSQL := "CREATE TABLE [r].[t1] (\n  [new_col] INT NOT NULL\n)" // "id" column dropped
	if err := os.WriteFile(sqlPath, []byte(createTableSQL), 0644); err != nil {
		t.Fatalf("writefile: %v", err)
	}

	layout := fs.Layout{
		Objects: []*fs.Object{{
			CachedFile:    fs.CachedFile{AbsPath: sqlPath},
			Path:          "r/tables/t1.sql",
			NormalizedKey: "r/tables/t1",
			Kind:          "tables",
			ObjectName:    "t1",
			SchemaName:    "r",
		}},
	}
	plan := &types.MigrationPlan{
		Blocked: true,
		Objects: []types.PlannedObject{{
			ObjectRef:     types.ObjectRef{NormalizedKey: "r/tables/t1", Kind: "tables", ObjectName: "t1", SchemaName: "r"},
			PlannedAction: types.ActionReprocessChangedBlocked,
		}},
	}

	dbColumns := map[string][]db.TableColumn{
		"r/tables/t1": {
			{Name: "id", TypeName: "INT", Nullable: false},
			{Name: "new_col", TypeName: "INT", Nullable: false},
		},
	}

	cfg := types.Config{SQLBase: baseDir}
	created, err := s.EnsureTransitionFiles(context.Background(), cfg, layout, plan, dbColumns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Fatal("expected created=true, fallback to scaffold")
	}

	dir := filepath.Join(baseDir, "r", "tables", "_migrations", "t1")
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			data, _ := os.ReadFile(filepath.Join(dir, e.Name()))
			if strings.Contains(string(data), "-- rmig: transition-scaffold") {
				return
			}
		}
	}
	t.Error("expected scaffold fallback when columns are dropped")
}
