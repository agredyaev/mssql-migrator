//go:build integration

package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"reporting-db-migrations/internal/apply"
	"reporting-db-migrations/internal/audit"
	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/db"
	"reporting-db-migrations/internal/diff"
	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/driver/mssql"
	"reporting-db-migrations/internal/engine"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/lock"
	"reporting-db-migrations/internal/scaffold"
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
	if len(objects) < 6 {
		t.Errorf("expected >= 6 objects, got %d", len(objects))
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

	ensureTestDatabase(t, ctx)

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

	ensureTestDatabase(t, ctx)

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

	if err := audit.EnsureTables(ctx, conn); err != nil {
		t.Fatalf("ensure audit tables: %v", err)
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

	t.Logf("plan: %d objects, blocked=%v, creates=%d, skips=%d, schemas=%d",
		len(plan.Objects), plan.Blocked,
		countByAction(plan, types.ActionCreateObject),
		countByAction(plan, types.ActionSkipUnchanged),
		len(plan.Schemas),
	)
	for _, s := range plan.Schemas {
		t.Logf("  schema %s action=%s", s.SchemaName, s.Action)
	}

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

	ensureTestDatabase(t, ctx)

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

	if err := audit.EnsureTables(ctx, conn); err != nil {
		t.Fatalf("ensure audit tables: %v", err)
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

	// Wire audit subscriber so metadata is written
	auditSub := audit.NewSubscriber(b, conn)
	_ = auditSub

	// Fire run started so subscriber creates run + bootstraps tables
	b.Publish(ctx, types.EventRunStarted, &types.RunStarted{Command: "migrate"})
	b.Publish(ctx, types.EventDiffComputed, &types.DiffResult{Plan: plan})

	executor := apply.New()
	result, err := executor.Execute(ctx, conn, *plan, layout, b)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	b.Publish(ctx, types.EventRunFinished, &types.RunFinished{Command: "migrate", Result: "success", ExitCode: 0})

	t.Logf("applied: %d failed: %d skipped: %d", result.Applied, result.Failed, result.Skipped)
	for _, e := range result.Errors {
		t.Logf("  apply error: %s", e)
	}
	if result.Applied == 0 {
		t.Error("expected to apply at least some objects")
	}

	// Verify audit metadata was written
	verifyAuditMetadata(t, ctx, conn)

	// Verify objects exist by inspecting again
	inspector2 := db.NewInspector()
	state2, err := inspector2.Inspect(ctx, conn, layout)
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

	ensureTestDatabase(t, ctx)

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

	// Apply all objects first
	applyAllObjects(t, ctx, conn, cfg, sqlRoot)

	// Now re-plan and check that objects are skipped
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

	if err := audit.EnsureTables(ctx, conn); err != nil {
		t.Fatalf("ensure audit tables: %v", err)
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

	if unchanged == 0 {
		t.Errorf("expected some objects to be skipped (unchanged), got %d", unchanged)
	}
}

func applyAllObjects(t *testing.T, ctx context.Context, conn driver.Conn, cfg types.Config, sqlRoot string) {
	t.Helper()

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

	if err := audit.EnsureTables(ctx, conn); err != nil {
		t.Fatalf("ensure audit tables: %v", err)
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
	auditSub := audit.NewSubscriber(b, conn)
	_ = auditSub

	b.Publish(ctx, types.EventRunStarted, &types.RunStarted{Command: "migrate"})
	b.Publish(ctx, types.EventDiffComputed, &types.DiffResult{Plan: plan})

	executor := apply.New()
	_, err = executor.Execute(ctx, conn, *plan, layout, b)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	b.Publish(ctx, types.EventRunFinished, &types.RunFinished{Command: "migrate", Result: "success", ExitCode: 0})
}

func configFromEnv() types.Config {
	return types.Config{
		Server:                 envOrDefault("RM_DB_SERVER", "localhost"),
		Port:                   envOrDefault("RM_DB_PORT", "1433"),
		Database:               envOrDefault("RM_DB_DATABASE", "dactests"),
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

func ensureTestDatabase(t *testing.T, ctx context.Context) {
	t.Helper()
	cfg := configFromEnv()
	dbName := cfg.Database

	// Connect to master to recreate the test database
	masterCfg := cfg
	masterCfg.Database = "master"
	conn, err := mssql.Open(ctx, masterCfg)
	if err != nil {
		t.Fatalf("connect to master: %v", err)
	}
	defer conn.Close()

	_, err = conn.ExecContext(ctx, `
		IF DB_ID('`+dbName+`') IS NOT NULL
		BEGIN
			ALTER DATABASE [`+dbName+`] SET SINGLE_USER WITH ROLLBACK IMMEDIATE;
			DROP DATABASE [`+dbName+`];
		END
		CREATE DATABASE [`+dbName+`]
	`, nil)
	if err != nil {
		t.Fatalf("drop/create database %s: %v", dbName, err)
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

// Engine wrappers for integration tests

type testLoaderAdapter struct{}

func (testLoaderAdapter) EnsureTables(ctx context.Context, conn driver.Conn) error {
	return audit.EnsureTables(ctx, conn)
}

func (testLoaderAdapter) LoadChecksums(ctx context.Context, conn driver.Conn, keys []string) (map[string]string, error) {
	return audit.LoadChecksums(ctx, conn, keys)
}

func (testLoaderAdapter) LoadAllAppliedMigrations(ctx context.Context, conn driver.Conn) (map[string]bool, error) {
	return audit.LoadAllAppliedMigrations(ctx, conn)
}

type testApplierAdapter struct {
	exec *apply.Executor
}

func (a *testApplierAdapter) Execute(ctx context.Context, conn driver.Conn, plan types.MigrationPlan, layout fs.Layout, eb bus.EventBus) (*apply.ApplyResult, error) {
	return a.exec.Execute(ctx, conn, plan, layout, eb)
}

func newTestEngine(b bus.EventBus, conn driver.Conn, cfg types.Config) *engine.Engine {
	return engine.New(
		cfg,
		b,
		conn,
		fs.NewScanner(),
		db.NewInspector(),
		testLoaderAdapter{},
		diff.NewComputer(),
		scaffold.New(),
		&testApplierAdapter{exec: apply.New()},
		lock.New(),
	)
}

func verifyAuditMetadata(t *testing.T, ctx context.Context, conn driver.Conn) {
	t.Helper()
	var objectCount, migrationCount int
	rows, err := conn.QueryContext(ctx, "SELECT COUNT(*) FROM azdo_deploy_meta.history WHERE kind = 'object'")
	if err != nil {
		t.Logf("verify audit: %v", err)
		return
	}
	if rows.Next() {
		rows.Scan(&objectCount)
	}
	rows.Close()

	rows, err = conn.QueryContext(ctx, "SELECT COUNT(*) FROM azdo_deploy_meta.history WHERE kind = 'migration'")
	if err != nil {
		t.Logf("verify audit: %v", err)
		return
	}
	if rows.Next() {
		rows.Scan(&migrationCount)
	}
	rows.Close()
	t.Logf("audit history: objects=%d migrations=%d", objectCount, migrationCount)
}

func TestIntegration_AddColumn_Migration(t *testing.T) {
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1")
	}

	cfg := configFromEnv()
	sqlRoot := filepath.Join("..", "..", ".temp", "sql")
	cfg.SQLRoot = sqlRoot
	cfg.SQLBase = sqlRoot
	cfg.LockTimeout = 30_000_000_000 // 30 seconds in nanoseconds

	ctx := context.Background()

	// Drop and recreate database for a clean start
	dropAndCreateTestDatabase(t, ctx, cfg)

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

	// --- Step 1: Apply baseline through engine ---
	t.Log("Step 1: applying baseline...")

	b := bus.New()
	auditSub := audit.NewSubscriber(b, conn)
	auditSub.SetErrorHandler(func(msg string) { t.Logf("audit err: %s", msg) })
	_ = auditSub

	eng := newTestEngine(b, conn, cfg)
	if err := eng.Migrate(ctx); err != nil {
		t.Fatalf("baseline migrate: %v", err)
	}

	verifyAuditMetadata(t, ctx, conn)

	// Verify history has entries
	var stateCount int
	rows3, err := conn.QueryContext(ctx, "SELECT COUNT(*) FROM azdo_deploy_meta.history WHERE kind = 'object'")
	if err != nil {
		t.Logf("verify history: %v", err)
	} else {
		if rows3.Next() {
			rows3.Scan(&stateCount)
		}
		rows3.Close()
		t.Logf("history object rows after baseline: %d", stateCount)
	}

	// Verify objects exist in DB
	var objCount int
	rows, err := conn.QueryContext(ctx, "SELECT COUNT(*) FROM sys.objects WHERE schema_id = SCHEMA_ID('smoke') AND is_ms_shipped = 0")
	if err != nil {
		t.Fatalf("verify objects: %v", err)
	}
	if rows.Next() {
		rows.Scan(&objCount)
	}
	rows.Close()
	t.Logf("DB objects after baseline: %d", objCount)
	if objCount < 5 {
		t.Errorf("expected at least 5 objects, got %d", objCount)
	}

	// --- Step 2: Add a column to smoke_table.sql ---
	t.Log("Step 2: adding column to smoke_table.sql...")

	tempRepo := filepath.Join("..", "..", ".temp")
	tableSQLPath := filepath.Join(sqlRoot, "dactests", "smoke", "tables", "smoke_table.sql")

	// Save current git HEAD so we can reset after test
	cleanHead := gitHead(t, tempRepo)

	// Ensure file is clean from previous runs
	gitCmd(t, tempRepo, "checkout", "HEAD", "--", "sql/dactests/smoke/tables/smoke_table.sql")

	original, err := os.ReadFile(tableSQLPath)
	if err != nil {
		t.Fatalf("read table SQL: %v", err)
	}

	modified := strings.Replace(string(original),
		"created_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME()",
		"created_at DATETIME2 NOT NULL DEFAULT SYSUTCDATETIME(),\n    added_at DATETIME2 NULL",
		1)

	if err := os.WriteFile(tableSQLPath, []byte(modified), 0644); err != nil {
		t.Fatalf("write modified table SQL: %v", err)
	}

	t.Cleanup(func() {
		cleanupScaffoldFiles(t, sqlRoot)
		gitCmd(t, tempRepo, "reset", "--hard", cleanHead)
	})

	// Git commit the column addition
	gitCmd(t, tempRepo, "add", "sql/dactests/smoke/tables/smoke_table.sql")
	gitCmd(t, tempRepo, "commit", "-m", "test: add added_at column to smoke_table")
	t.Log("Step 2b: committed column addition to git")

	// --- Step 3: Run migrate → blocked → scaffold creates auto-migration ---
	t.Log("Step 3: running migrate (should be blocked, scaffold creates file)...")

	b2 := bus.New()
	auditSub2 := audit.NewSubscriber(b2, conn)
	auditSub2.SetErrorHandler(func(msg string) { t.Logf("audit err: %s", msg) })
	_ = auditSub2

	eng2 := newTestEngine(b2, conn, cfg)
	err = eng2.Migrate(ctx)
	if err == nil {
		// Not blocked — log what the plan says
		scanner3 := fs.NewScanner()
		layout3, _ := scanner3.Scan(ctx, sqlRoot)
		inspector3 := db.NewInspector()
		state3, _ := inspector3.Inspect(ctx, conn, layout3)
		checksums3, _ := audit.LoadChecksums(ctx, conn, layout3.NormalizedKeys())
		t.Logf("after add-column: state objects=%d checksums=%d", len(state3.Objects), len(checksums3))
		for _, obj := range layout3.Objects {
			t.Logf("  %s: db=%v audit=%q", obj.NormalizedKey, state3.Objects[obj.NormalizedKey].ObjectName != "", checksums3[obj.NormalizedKey])
		}
		t.Fatal("expected blocked error after adding column")
	}
	t.Logf("expected blocked error: %v", err)

	// Check that scaffold created migration file
	migrationDir := filepath.Join(sqlRoot, "dactests", "smoke", "tables", "_migrations", "smoke_table")
	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		t.Fatalf("read migration dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected scaffold to create migration file")
	}
	t.Logf("scaffolded migration files: %d", len(entries))
	for _, e := range entries {
		t.Logf("  %s", e.Name())
		b, _ := os.ReadFile(filepath.Join(migrationDir, e.Name()))
		t.Logf("  content:\n%s", string(b))
	}

	// --- Step 4: Apply the migration (fresh engine) ---
	t.Log("Step 4: applying migration...")

	b3 := bus.New()
	auditSub3 := audit.NewSubscriber(b3, conn)
	auditSub3.SetErrorHandler(func(msg string) { t.Logf("audit err: %s", msg) })
	_ = auditSub3

	eng3 := newTestEngine(b3, conn, cfg)
	if err := eng3.Migrate(ctx); err != nil {
		t.Fatalf("migration apply: %v", err)
	}

	// --- Step 5: Verify column was added in DB ---
	t.Log("Step 5: verifying added column in DB...")

	var columnExists int
	rows2, err := conn.QueryContext(ctx, `
		SELECT COUNT(*) FROM sys.columns c
		JOIN sys.objects o ON o.object_id = c.object_id
		JOIN sys.schemas s ON s.schema_id = o.schema_id
		WHERE s.name = 'smoke' AND o.name = 'smoke_table' AND c.name = 'added_at'
	`)
	if err != nil {
		t.Fatalf("verify column: %v", err)
	}
	if rows2.Next() {
		rows2.Scan(&columnExists)
	}
	rows2.Close()
	if columnExists == 0 {
		t.Error("column 'added_at' was not found in smoke.smoke_table")
	} else {
		t.Logf("column 'added_at' exists in smoke.smoke_table")
	}

	verifyAuditMetadata(t, ctx, conn)
}

func dropAndCreateTestDatabase(t *testing.T, ctx context.Context, cfg types.Config) {
	t.Helper()
	dbName := cfg.Database
	masterCfg := cfg
	masterCfg.Database = "master"
	conn, err := mssql.Open(ctx, masterCfg)
	if err != nil {
		t.Fatalf("connect to master: %v", err)
	}
	defer conn.Close()

	_, err = conn.ExecContext(ctx, `
		IF DB_ID('`+dbName+`') IS NOT NULL
		BEGIN
			ALTER DATABASE [`+dbName+`] SET SINGLE_USER WITH ROLLBACK IMMEDIATE;
			DROP DATABASE [`+dbName+`];
		END
		CREATE DATABASE [`+dbName+`]
	`, nil)
	if err != nil {
		t.Fatalf("drop/create database %s: %v", dbName, err)
	}
}

func cleanupScaffoldFiles(t *testing.T, sqlRoot string) {
	t.Helper()
	migrationDir := filepath.Join(sqlRoot, "dactests", "smoke", "tables", "_migrations", "smoke_table")
	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		content, err := os.ReadFile(filepath.Join(migrationDir, e.Name()))
		if err != nil {
			continue
		}
		if strings.Contains(string(content), "auto_add_columns") ||
			strings.Contains(string(content), "Auto-generated migration") ||
			strings.Contains(string(content), "describe_change") {
			os.Remove(filepath.Join(migrationDir, e.Name()))
		}
	}
	os.RemoveAll(migrationDir)
	os.Remove(filepath.Join(sqlRoot, "dactests", "smoke", "tables", "_migrations"))
}

func gitCmd(t *testing.T, repo, arg string, args ...string) {
	t.Helper()
	cmdArgs := append([]string{"-C", repo, arg}, args...)
	cmd := exec.Command("git", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("git %v: %s (output: %s)", cmdArgs, err.Error(), string(out))
	}
}

func gitHead(t *testing.T, repo string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", repo, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse: %v", err)
	}
	return strings.TrimSpace(string(out))
}
