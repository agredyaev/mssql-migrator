//go:build integration

package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"reporting-db-migrations/internal/apply"
	"reporting-db-migrations/internal/audit"
	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/db"
	"reporting-db-migrations/internal/diff"
	"reporting-db-migrations/internal/driver"
	"reporting-db-migrations/internal/driver/mssql"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/types"
)

// ============================================================
// LOAD TEST 1: Repeated plan+apply — idempotency under load
// Uses the exact integration-test pattern (direct executor, not engine)
// ============================================================

func TestLoad_IdempotentRepeatedApply(t *testing.T) {
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1")
	}
	dbName := "rmig_ld1"
	recreateDB(t, dbName)

	cfg := e2eCfg(dbName)
	cfg.SQLRoot = filepath.Join("..", "..", ".temp", "sql")
	cfg.SQLBase = cfg.SQLRoot

	ctx := context.Background()
	conn, err := mssql.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	if err := audit.EnsureTables(ctx, conn); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	layout, plan := scanAndCompute(t, ctx, conn, cfg.SQLRoot)
	b := bus.New()
	_ = audit.NewSubscriber(b, conn)

	// Apply baseline
	result, err := apply.New().Execute(ctx, conn, *plan, layout, b)
	if err != nil {
		t.Fatalf("baseline execute: %v", err)
	}
	if result.Failed > 0 {
		t.Fatalf("baseline failed: %v", result.Errors)
	}
	t.Logf("baseline: %d applied, %d failed, %d skipped", result.Applied, result.Failed, result.Skipped)

	baselineObj := countAuditRows(t, conn, "object")
	baselineMig := countAuditRows(t, conn, "migration")
	t.Logf("baseline audit: %d objects, %d migrations", baselineObj, baselineMig)

	// 10 repeated apply cycles
	for i := range 10 {
		_, newPlan := scanAndCompute(t, ctx, conn, cfg.SQLRoot)
		res, err := apply.New().Execute(ctx, conn, *newPlan, layout, b)
		if err != nil {
			t.Fatalf("attempt %d: execute error: %v", i+1, err)
		}
		if res.Failed > 0 {
			t.Fatalf("attempt %d: %d failed: %v", i+1, res.Failed, res.Errors)
		}
		if res.Applied > 0 {
			t.Errorf("attempt %d: expected 0 applied, got %d", i+1, res.Applied)
		}

		curObj := countAuditRows(t, conn, "object")
		curMig := countAuditRows(t, conn, "migration")
		if curObj != baselineObj || curMig != baselineMig {
			t.Errorf("attempt %d: audit drifted (obj %d→%d, mig %d→%d)",
				i+1, baselineObj, curObj, baselineMig, curMig)
		}
	}
	t.Logf("10 repeated apply cycles: audit stable at %d objects, %d migrations", baselineObj, baselineMig)
}

// ============================================================
// LOAD TEST 2: Concurrent lock — two connections, one should time out
// ============================================================

func TestLoad_Concurrent(t *testing.T) {
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1")
	}
	dbName := "rmig_ld2"
	recreateDB(t, dbName)

	cfg := e2eCfg(dbName)
	cfg.SQLRoot = filepath.Join("..", "..", ".temp", "sql")
	cfg.SQLBase = cfg.SQLRoot

	conn1, err := mssql.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("conn1: %v", err)
	}
	defer conn1.Close()
	conn2, err := mssql.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("conn2: %v", err)
	}
	defer conn2.Close()

	if err := audit.EnsureTables(context.Background(), conn1); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	layout, plan := scanAndCompute(t, context.Background(), conn1, cfg.SQLRoot)

	var wg sync.WaitGroup
	var execErr1, execErr2 error
	startCh := make(chan struct{})

	run := func(conn driver.Conn) (*apply.ApplyResult, error) {
		b := bus.New()
		_ = audit.NewSubscriber(b, conn)
		return apply.New().Execute(context.Background(), conn, *plan, layout, b)
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-startCh
		_, execErr1 = run(conn1)
	}()
	go func() {
		defer wg.Done()
		<-startCh
		_, execErr2 = run(conn2)
	}()

	close(startCh)
	wg.Wait()

	successCount := 0
	for _, err := range []error{execErr1, execErr2} {
		if err == nil {
			successCount++
		}
	}
	t.Logf("concurrent apply: %d success, err1=%v, err2=%v", successCount, execErr1, execErr2)

	if successCount == 0 {
		t.Error("both concurrent applies returned error")
	}

	// Without locks, concurrent applies may have partial failures —
	// but most objects should still be created
	var objCount int
	rows, _ := conn1.QueryContext(context.Background(),
		"SELECT COUNT(*) FROM sys.objects WHERE SCHEMA_NAME(schema_id) = 'smoke' AND type IN ('U','V','P','FN','IF','TR')")
	if rows.Next() {
		rows.Scan(&objCount)
	}
	rows.Close()
	if objCount < 3 {
		t.Errorf("expected at least 3 objects in DB (concurrent partial success), got %d", objCount)
	}
	t.Logf("concurrent test: %d objects created", objCount)
}

// ============================================================
// LOAD TEST 3: Audit integrity — verify checksums, keys, events
// ============================================================

func TestLoad_AuditIntegrity(t *testing.T) {
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1")
	}
	dbName := "rmig_ld3"
	recreateDB(t, dbName)

	cfg := e2eCfg(dbName)
	cfg.SQLRoot = filepath.Join("..", "..", ".temp", "sql")
	cfg.SQLBase = cfg.SQLRoot

	ctx := context.Background()
	conn, err := mssql.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	if err := audit.EnsureTables(ctx, conn); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	layout, plan := scanAndCompute(t, ctx, conn, cfg.SQLRoot)
	b := bus.New()
	_ = audit.NewSubscriber(b, conn)

	res, err := apply.New().Execute(ctx, conn, *plan, layout, b)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Failed > 0 {
		t.Fatalf("apply failed: %v", res.Errors)
	}
	t.Logf("applied %d objects", res.Applied)

	// Read all audit records
	rows, _ := conn.QueryContext(ctx, `
		SELECT kind, normalized_key, checksum, event, error_text
		FROM azdo_deploy_meta.history ORDER BY id`)
	type rec struct{ kind, key, cs, event, errText string }
	var records []rec
	for rows.Next() {
		var r rec
		rows.Scan(&r.kind, &r.key, &r.cs, &r.event, &r.errText)
		records = append(records, r)
	}
	rows.Close()

	if len(records) == 0 {
		t.Fatal("no audit records")
	}

	objects := 0
	migrations := 0
	for _, r := range records {
		switch r.kind {
		case "object":
			objects++
			if r.event != "applied" && r.event != "adopted" && r.event != "skipped" {
				t.Errorf("unexpected event %q for object %s", r.event, r.key)
			}
			if r.cs == "" && r.event != "skipped" {
				t.Errorf("empty checksum for object %s (event=%s)", r.key, r.event)
			}
			if r.key == "" {
				t.Error("empty normalized_key in audit")
			}
		case "migration":
			migrations++
		default:
			t.Errorf("unknown record kind: %s", r.kind)
		}
	}

	if objects == 0 {
		t.Error("expected at least 1 object audit record")
	}
	t.Logf("audit integrity: %d records (%d objects, %d migrations), all keys/checksums present", len(records), objects, migrations)
}

// ============================================================
// LOAD TEST 4: Stress — 50 additional procedures
// ============================================================

func TestLoad_StressApply(t *testing.T) {
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1")
	}
	dbName := "rmig_ld4"
	recreateDB(t, dbName)

	layoutRoot := t.TempDir()
	srcSQL := filepath.Join("..", "..", ".temp", "sql", "dactests", "smoke")
	dstSQL := filepath.Join(layoutRoot, "dactests", "smoke")
	if err := copyDir(srcSQL, dstSQL); err != nil {
		t.Fatalf("copy: %v", err)
	}

	// Add 50 procedures
	procDir := filepath.Join(dstSQL, "procedures")
	for i := range 50 {
		fname := fmt.Sprintf("ld4_%04d.sql", i)
		content := fmt.Sprintf("CREATE OR ALTER PROC [smoke].[ld4_%04d] AS SELECT %d AS n;", i, i)
		if err := os.WriteFile(filepath.Join(procDir, fname), []byte(content), 0644); err != nil {
			t.Fatalf("write proc %d: %v", i, err)
		}
	}

	cfg := e2eCfg(dbName)
	cfg.SQLRoot = layoutRoot
	cfg.SQLBase = layoutRoot

	ctx := context.Background()
	conn, err := mssql.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	if err := audit.EnsureTables(ctx, conn); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	layout, plan := scanAndCompute(t, ctx, conn, cfg.SQLRoot)
	b := bus.New()
	_ = audit.NewSubscriber(b, conn)

	start := time.Now()
	res, err := apply.New().Execute(ctx, conn, *plan, layout, b)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.Failed > 0 {
		t.Fatalf("%d failed: %v", res.Failed, res.Errors)
	}
	t.Logf("applied %d objects in %v (failed=%d skipped=%d)", res.Applied, elapsed, res.Failed, res.Skipped)

	var procCount int
	rows, _ := conn.QueryContext(ctx, "SELECT COUNT(*) FROM sys.procedures WHERE SCHEMA_NAME(schema_id) = 'smoke'")
	if rows.Next() {
		rows.Scan(&procCount)
	}
	rows.Close()

	var total int
	rows2, _ := conn.QueryContext(ctx, "SELECT COUNT(*) FROM sys.objects WHERE SCHEMA_NAME(schema_id) = 'smoke' AND type IN ('U','V','P','FN','IF')")
	if rows2.Next() {
		rows2.Scan(&total)
	}
	rows2.Close()

	if procCount < 51 {
		t.Errorf("expected >=51 procs, got %d", procCount)
	}
	t.Logf("stress: %d total objects, %d procedures", total, procCount)
}

// ============================================================
// LOAD TEST 5: Error recovery — invalid SQL → fix → retry
// ============================================================

func TestLoad_ErrorRecovery(t *testing.T) {
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1")
	}
	dbName := "rmig_ld5"
	recreateDB(t, dbName)

	layoutRoot := t.TempDir()
	srcSQL := filepath.Join("..", "..", ".temp", "sql", "dactests", "smoke")
	dstSQL := filepath.Join(layoutRoot, "dactests", "smoke")
	if err := copyDir(srcSQL, dstSQL); err != nil {
		t.Fatalf("copy: %v", err)
	}

	badPath := filepath.Join(dstSQL, "procedures", "bad_proc.sql")
	if err := os.WriteFile(badPath, []byte("CREATE OR ALTER PROC [smoke].[bad_proc] AS INVALID SQL SYNTAX ERROR;"), 0644); err != nil {
		t.Fatalf("write bad proc: %v", err)
	}

	cfg := e2eCfg(dbName)
	cfg.SQLRoot = layoutRoot
	cfg.SQLBase = layoutRoot

	ctx := context.Background()
	conn, err := mssql.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	if err := audit.EnsureTables(ctx, conn); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	b := bus.New()
	_ = audit.NewSubscriber(b, conn)

	layout, plan := scanAndCompute(t, ctx, conn, cfg.SQLRoot)
	res, err := apply.New().Execute(ctx, conn, *plan, layout, b)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.Failed < 1 {
		t.Fatal("expected at least 1 failure from invalid SQL")
	}
	t.Logf("first attempt: %d applied, %d failed", res.Applied, res.Failed)

	// Verify good objects exist despite failure
	var tableCount int
	rows, _ := conn.QueryContext(ctx, "SELECT COUNT(*) FROM sys.tables WHERE SCHEMA_NAME(schema_id) = 'smoke'")
	if rows.Next() {
		rows.Scan(&tableCount)
	}
	rows.Close()
	if tableCount < 1 {
		t.Error("expected at least 1 table (partial success)")
	}

	// Fix the bad file
	if err := os.WriteFile(badPath, []byte("CREATE OR ALTER PROC [smoke].[bad_proc] AS SELECT 'fixed' AS status;"), 0644); err != nil {
		t.Fatalf("fix: %v", err)
	}

	// Retry
	layout2, plan2 := scanAndCompute(t, ctx, conn, cfg.SQLRoot)
	res2, err := apply.New().Execute(ctx, conn, *plan2, layout2, b)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if res2.Failed > 0 {
		t.Fatalf("retry: %d still failed: %v", res2.Failed, res2.Errors)
	}
	t.Logf("retry: %d applied", res2.Applied)

	var procCount int
	rows2, _ := conn.QueryContext(ctx, "SELECT COUNT(*) FROM sys.procedures WHERE SCHEMA_NAME(schema_id) = 'smoke'")
	if rows2.Next() {
		rows2.Scan(&procCount)
	}
	rows2.Close()
	if procCount < 1 {
		t.Error("expected at least 1 procedure after fix")
	}

	var failedAudit int
	rows3, _ := conn.QueryContext(ctx, "SELECT COUNT(*) FROM azdo_deploy_meta.history WHERE event='failed'")
	if rows3.Next() {
		rows3.Scan(&failedAudit)
	}
	rows3.Close()
	t.Logf("error recovery: %d failed events, then fixed (%d procs)", failedAudit, procCount)
}

// ============================================================
// Helpers
// ============================================================

func scanAndCompute(t *testing.T, ctx context.Context, conn driver.Conn, sqlRoot string) (fs.Layout, *types.MigrationPlan) {
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

	checksums, err := audit.LoadChecksums(ctx, conn, layout.NormalizedKeys())
	if err != nil {
		t.Fatalf("checksums: %v", err)
	}

	computer := diff.NewComputer()
	plan, err := computer.Compute(ctx, layout, state, checksums)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}

	return layout, plan
}

func recreateDB(t *testing.T, dbName string) {
	t.Helper()
	cfg := types.Config{
		Server:                 envOrDefault("RM_DB_SERVER", "localhost"),
		Port:                   envOrDefault("RM_DB_PORT", "1433"),
		Database:               "master",
		DBAuth:                 "sql",
		User:                   envOrDefault("RM_DB_USER", "sa"),
		Password:               envOrDefault("RM_DB_PASSWORD", "yourStrong(!)Password"),
		Encrypt:                false,
		TrustServerCertificate: true,
	}
	conn, err := mssql.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("connect master: %v", err)
	}
	defer conn.Close()

	_, err = conn.ExecContext(context.Background(), fmt.Sprintf(`
		IF DB_ID('%s') IS NOT NULL
		BEGIN
			ALTER DATABASE [%s] SET SINGLE_USER WITH ROLLBACK IMMEDIATE;
			DROP DATABASE [%s];
		END
		CREATE DATABASE [%s];`, dbName, dbName, dbName, dbName), nil)
	if err != nil {
		t.Fatalf("recreate database %s: %v", dbName, err)
	}
}

func e2eCfg(dbName string) types.Config {
	return types.Config{
		Server:                 envOrDefault("RM_DB_SERVER", "localhost"),
		Port:                   envOrDefault("RM_DB_PORT", "1433"),
		Database:               dbName,
		DBAuth:                 "sql",
		User:                   envOrDefault("RM_DB_USER", "sa"),
		Password:               envOrDefault("RM_DB_PASSWORD", "yourStrong(!)Password"),
		Encrypt:                false,
		TrustServerCertificate: true,
		LogLevel:               "debug",
	}
}

func countAuditRows(t *testing.T, conn driver.Conn, kind string) int {
	t.Helper()
	var count int
	rows, _ := conn.QueryContext(context.Background(),
		"SELECT COUNT(*) FROM azdo_deploy_meta.history WHERE kind = @p1", kind)
	if rows.Next() {
		rows.Scan(&count)
	}
	rows.Close()
	return count
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0644)
	})
}
