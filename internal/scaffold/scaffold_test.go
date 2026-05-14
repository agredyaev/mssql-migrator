package scaffold

import (
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
	plan := types.MigrationPlan{
		Objects: []types.PlannedObject{{
			NormalizedKey: "r/views/v1",
			PlannedAction: types.ActionCreateObject,
		}},
	}
	columns := map[string][]db.TableColumn{}

	paths, err := s.EnsureTransitionFiles("", layout, plan, columns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("expected 0 paths, got %d", len(paths))
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
	plan := types.MigrationPlan{
		Blocked: true,
		Objects: []types.PlannedObject{{
			NormalizedKey: "r/tables/t1",
			PlannedAction: types.ActionReprocessChangedBlocked,
			Kind:          "tables",
			ObjectName:    "t1",
			SchemaName:    "r",
		}},
	}
	columns := map[string][]db.TableColumn{
		"r/tables/t1": {
			{Name: "id", TypeName: "int", Nullable: false},
			{Name: "name", TypeName: "nvarchar", Nullable: false},
		},
	}

	paths, err := s.EnsureTransitionFiles(baseDir, layout, plan, columns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}

	fullPath := filepath.Join(baseDir, paths[0])
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("failed to read generated file: %v", err)
	}
	content := string(data)
	if content == "" {
		t.Fatal("generated file is empty")
	}
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
	plan := types.MigrationPlan{
		Blocked: true,
		Objects: []types.PlannedObject{{
			NormalizedKey: "r/tables/t1",
			PlannedAction: types.ActionReprocessChangedBlocked,
			Kind:          "tables",
			ObjectName:    "t1",
			SchemaName:    "r",
		}},
	}
	columns := map[string][]db.TableColumn{}

	paths1, err := s.EnsureTransitionFiles(baseDir, layout, plan, columns)
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}
	paths2, err := s.EnsureTransitionFiles(baseDir, layout, plan, columns)
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if len(paths1) != 1 {
		t.Fatalf("first call: expected 1 path, got %d", len(paths1))
	}
	if len(paths2) != 0 {
		t.Errorf("second call: expected 0 paths (idempotent), got %d", len(paths2))
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
	plan := types.MigrationPlan{
		Blocked: true,
		Objects: []types.PlannedObject{{
			NormalizedKey: "r/tables/t1",
			PlannedAction: types.ActionReprocessChangedBlocked,
			Kind:          "tables",
			ObjectName:    "t1",
			SchemaName:    "r",
		}},
	}
	columns := map[string][]db.TableColumn{}

	paths, err := s.EnsureTransitionFiles(baseDir, layout, plan, columns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("expected 0 paths (real transition exists), got %d", len(paths))
	}
}
