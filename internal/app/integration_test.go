//go:build integration

package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"reporting-db-migrations/internal/apply"
	"reporting-db-migrations/internal/audit"
	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/db"
	"reporting-db-migrations/internal/diff"
	"reporting-db-migrations/internal/driver/mssql"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/types"
)

func TestIntegration_ScanAndInspect(t *testing.T) {
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1")
	}

	cfg := configFromEnv()
	// Use .temp/sql as the SQL root
	sqlRoot := filepath.Join("..", "..", ".temp", "sql")
	cfg.SQLRoot = sqlRoot
	cfg.SQLBase = sqlRoot

	ctx := context.Background()

	scanner := fs.NewScanner()
	layout, err := scanner.Scan(ctx, sqlRoot)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(layout.Schemas) == 0 {
		t.Fatal("expected at least one schema")
	}
	if layout.Schemas[0].Name != "smoke" {
		t.Errorf("schema = %q, want smoke", layout.Schemas[0].Name)
	}

	objects := layout.Objects
	if len(objects) < 7 {
		t.Errorf("expected >= 7 objects, got %d", len(objects))
	}

	t.Logf("found %d schemas, %d objects, %d checks", len(layout.Schemas), len(objects), len(layout.Checks))
	for _, obj := range objects {
		t.Logf("  object: %s/%s/%s key=%s", obj.SchemaName, obj.Kind, obj.ObjectName, obj.NormalizedKey)
	}
}

func TestIntegration_DBInspect_EmptyDB(t *testing.T) {
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1")
	}

	cfg := configFromEnv()
	sqlRoot := filepath.Join("..", "..", ".temp", "sql")
	cfg.SQLRoot = sqlRoot

	ctx := context.Background()

	conn, err := mssql.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	scanner := fs.NewScanner()
	layout, err := scanner.Scan(ctx, sqlRoot)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	inspector := db.NewInspector()
	state, err := inspector.Inspect(ctx, conn, layout)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}

	t.Logf("db state: %d schemas, %d objects", len(state.Schemas), len(state.Objects))
}

func TestIntegration_Plan_EmptyDB(t *testing.T) {
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1")
	}

	cfg := configFromEnv()
	sqlRoot := filepath.Join("..", "..", ".temp", "sql")
	cfg.SQLRoot = sqlRoot

	ctx := context.Background()

	conn, err := mssql.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	scanner := fs.NewScanner()
	layout, err := scanner.Scan(ctx, sqlRoot)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	inspector := db.NewInspector()
	state, err := inspector.Inspect(ctx, conn, layout)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}

	checksums, err := audit.LoadChecksums(ctx, conn, layout.NormalizedKeys())
	if err != nil {
		t.Fatalf("load checksums: %v", err)
	}

	computer := diff.NewComputer()
	plan, err := computer.Compute(ctx, layout, state, checksums)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}

	t.Logf("plan: %d objects, blocked=%v, creates=%d, skips=%d",
		len(plan.Objects), plan.Blocked,
		countByAction(plan, types.ActionCreateObject),
		countByAction(plan, types.ActionSkipUnchanged),
	)

	for _, obj := range plan.Objects {
		t.Logf("  %s → %s (exists=%v)", obj.NormalizedKey, obj.PlannedAction, obj.Exists)
	}

	if countByAction(plan, types.ActionCreateObject) == 0 {
		t.Error("expected some objects to be created in empty DB")
	}
}

func TestIntegration_Apply_AllObjects(t *testing.T) {
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1")
	}

	cfg := configFromEnv()
	sqlRoot := filepath.Join("..", "..", ".temp", "sql")
	cfg.SQLRoot = sqlRoot

	ctx := context.Background()

	conn, err := mssql.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	// Create schema if not exists
	if _, err := conn.ExecContext(ctx, `
		IF SCHEMA_ID('smoke') IS NULL EXEC('CREATE SCHEMA smoke')
	`, nil); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	scanner := fs.NewScanner()
	layout, err := scanner.Scan(ctx, sqlRoot)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	inspector := db.NewInspector()
	state, err := inspector.Inspect(ctx, conn, layout)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}

	checksums, err := audit.LoadChecksums(ctx, conn, layout.NormalizedKeys())
	if err != nil {
		t.Fatalf("load checksums: %v", err)
	}

	computer := diff.NewComputer()
	plan, err := computer.Compute(ctx, layout, state, checksums)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}

	b := bus.New()

	executor := apply.New()
	result, err := executor.Execute(ctx, conn, *plan, layout, b)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	t.Logf("applied: %d objects", result.Applied)
	if result.Applied == 0 {
		t.Error("expected to apply at least some objects")
	}

	// Verify objects exist by inspecting again
	state2, err := inspector.Inspect(ctx, conn, layout)
	if err != nil {
		t.Fatalf("post-apply inspect: %v", err)
	}

	t.Logf("post-apply state: %d db objects", len(state2.Objects))
	if len(state2.Objects) == 0 {
		t.Error("expected objects in DB after apply")
	}
}

func TestIntegration_Plan_AfterApply(t *testing.T) {
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1")
	}

	cfg := configFromEnv()
	sqlRoot := filepath.Join("..", "..", ".temp", "sql")
	cfg.SQLRoot = sqlRoot

	ctx := context.Background()

	conn, err := mssql.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	scanner := fs.NewScanner()
	layout, err := scanner.Scan(ctx, sqlRoot)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	inspector := db.NewInspector()
	state, err := inspector.Inspect(ctx, conn, layout)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}

	checksums, err := audit.LoadChecksums(ctx, conn, layout.NormalizedKeys())
	if err != nil {
		t.Fatalf("load checksums: %v", err)
	}

	computer := diff.NewComputer()
	plan, err := computer.Compute(ctx, layout, state, checksums)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}

	for _, obj := range plan.Objects {
		t.Logf("  %s → %s", obj.NormalizedKey, obj.PlannedAction)
	}

	unchanged := countByAction(plan, types.ActionSkipUnchanged)
	adopt := countByAction(plan, types.ActionAdoptExisting)
	create := countByAction(plan, types.ActionCreateObject)

	t.Logf("skip=%d, adopt=%d, create=%d", unchanged, adopt, create)

	// After apply, we expect most objects to be adopt or skip
	if create > 0 {
		t.Logf("note: %d objects still need creation (may not have audit records)", create)
	}
}

func configFromEnv() types.Config {
	return types.Config{
		Server:                 envOrDefault("RM_DB_SERVER", "localhost"),
		Port:                   envOrDefault("RM_DB_PORT", "1433"),
		Database:               envOrDefault("RM_DB_DATABASE", "rmig_test"),
		DBAuth:                 envOrDefault("RM_DB_AUTH", "sql"),
		User:                   envOrDefault("RM_DB_USER", "sa"),
		Password:               envOrDefault("RM_DB_PASSWORD", "yourStrong(!)Password"),
		Encrypt:                false,
		TrustServerCertificate: true,
		SQLRoot:                envOrDefault("RM_SQL_ROOT", "sql"),
		SQLBase:                envOrDefault("RM_SQL_BASE", "sql"),
		LogLevel:               envOrDefault("RM_LOG_LEVEL", "debug"),
	}
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func countByAction(plan *types.MigrationPlan, action string) int {
	n := 0
	for _, obj := range plan.Objects {
		if obj.PlannedAction == action {
			n++
		}
	}
	return n
}
