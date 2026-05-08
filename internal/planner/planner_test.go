package planner

import (
	"os"
	"path/filepath"
	"testing"

	"reporting-db-migrations/internal/checksum"
	"reporting-db-migrations/internal/config"
	"reporting-db-migrations/internal/state"
)

func TestBuildPlansPendingAndChangedRepeatable(t *testing.T) {
	root := t.TempDir()
	writeSQL(t, root, "versioned", "V001__create.sql", "SELECT 1;")
	writeSQL(t, root, "repeatable", "R001__views.sql", "SELECT 2;")

	cfg := config.Config{Env: "prod", Database: "ReportingDB", SQLDir: root, ToolVersion: "4.0.0"}
	migrationState := state.New([]state.Attempt{
		{ScriptName: "R001__views.sql", Checksum: "old", Success: true},
	})

	plan, err := Build(cfg, migrationState)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.PendingScripts) != 1 {
		t.Fatalf("expected one pending script, got %d", len(plan.PendingScripts))
	}
	if len(plan.ChangedRepeatableScripts) != 1 {
		t.Fatalf("expected one changed repeatable script, got %d", len(plan.ChangedRepeatableScripts))
	}
}

func TestBuildBlocksChangedVersionedScript(t *testing.T) {
	root := t.TempDir()
	path := writeSQL(t, root, "versioned", "V001__create.sql", "SELECT 1;")
	current, err := checksum.SHA256File(path)
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{Env: "prod", Database: "ReportingDB", SQLDir: root, ToolVersion: "4.0.0"}
	migrationState := state.New([]state.Attempt{
		{ScriptName: "V001__create.sql", Checksum: current + "changed", Success: true},
	})

	plan, err := Build(cfg, migrationState)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Blocked {
		t.Fatal("expected plan to be blocked")
	}
}

func writeSQL(t *testing.T, root string, folder string, name string, content string) string {
	t.Helper()
	dir := filepath.Join(root, folder)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
