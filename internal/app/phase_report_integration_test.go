//go:build integration

package app

import (
	"context"
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

// timingConn wraps driver.Conn and accumulates wall time spent inside
// Query*, ExecContext, and Ping. Time spent blocked in rows.Next() after
// Query returns is not attributed to the driver boundary here (documented
// limitation for interpreting "I/O" vs pure CPU in diff/scan).
type timingConn struct {
	inner      driver.Conn
	mu         sync.Mutex
	queryTotal time.Duration
	execTotal  time.Duration
	pingTotal  time.Duration
	queryCalls int64
	execCalls  int64
	pingCalls  int64
}

func newTimingConn(inner driver.Conn) *timingConn {
	return &timingConn{inner: inner}
}

func (c *timingConn) QueryContext(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	start := time.Now()
	rows, err := c.inner.QueryContext(ctx, query, args...)
	c.addQuery(time.Since(start))
	return rows, err
}

func (c *timingConn) QueryStringsContext(ctx context.Context, query string, args []string) (driver.Rows, error) {
	start := time.Now()
	rows, err := c.inner.QueryStringsContext(ctx, query, args)
	c.addQuery(time.Since(start))
	return rows, err
}

func (c *timingConn) QueryStringSlicesContext(ctx context.Context, query string, args1, args2 []string) (driver.Rows, error) {
	start := time.Now()
	rows, err := c.inner.QueryStringSlicesContext(ctx, query, args1, args2)
	c.addQuery(time.Since(start))
	return rows, err
}

func (c *timingConn) ExecContext(ctx context.Context, query string, args ...any) (driver.Result, error) {
	start := time.Now()
	res, err := c.inner.ExecContext(ctx, query, args...)
	c.addExec(time.Since(start))
	return res, err
}

func (c *timingConn) Ping(ctx context.Context) error {
	start := time.Now()
	err := c.inner.Ping(ctx)
	c.addPing(time.Since(start))
	return err
}

func (c *timingConn) Close() error {
	return c.inner.Close()
}

func (c *timingConn) addQuery(d time.Duration) {
	c.mu.Lock()
	c.queryTotal += d
	c.queryCalls++
	c.mu.Unlock()
}

func (c *timingConn) addExec(d time.Duration) {
	c.mu.Lock()
	c.execTotal += d
	c.execCalls++
	c.mu.Unlock()
}

func (c *timingConn) addPing(d time.Duration) {
	c.mu.Lock()
	c.pingTotal += d
	c.pingCalls++
	c.mu.Unlock()
}

func (c *timingConn) logSummary(t *testing.T) {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	dbBoundary := c.queryTotal + c.execTotal + c.pingTotal
	t.Logf("driver.Conn wall inside Query/Exec/Ping: %s (Query=%s n=%d, Exec=%s n=%d, Ping=%s n=%d)",
		dbBoundary, c.queryTotal, c.queryCalls, c.execTotal, c.execCalls, c.pingTotal, c.pingCalls)
	t.Logf("note: fetch/decode time inside rows.Next after Query returns is not included above")
}

// TestIntegration_PhaseReport_PlanPipeline prints per-phase wall times and
// driver-boundary DB time for the same steps as TestIntegration_Plan_EmptyDB.
// Run: make test-int-phase (see Makefile) or make test-int ARGS='-run TestIntegration_PhaseReport_PlanPipeline'.
func TestIntegration_PhaseReport_PlanPipeline(t *testing.T) {
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1")
	}

	cfg := configFromEnv()
	sqlRoot := filepath.Join("..", "..", ".temp", "sql")
	cfg.SQLRoot = sqlRoot

	ctx := context.Background()

	t0 := time.Now()
	ensureTestDatabase(t, ctx)
	t.Logf("phase ensureTestDatabase: %s", time.Since(t0))

	startConn := time.Now()
	raw, err := mssql.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	connectDur := time.Since(startConn)

	tc := newTimingConn(raw)
	defer func() { _ = tc.Close() }()

	t.Logf("phase connect (Open includes Ping): %s", connectDur)

	var scanDur time.Duration
	var layout fs.Layout
	{
		start := time.Now()
		scanner := fs.NewScanner()
		layout, err = scanner.Scan(ctx, sqlRoot)
		if err != nil {
			t.Fatalf("scan: %v", err)
		}
		scanDur = time.Since(start)
	}
	t.Logf("phase fs.Scan: %s", scanDur)

	var inspectDur time.Duration
	var state *db.State
	{
		start := time.Now()
		inspector := db.NewInspector()
		state, err = inspector.Inspect(ctx, tc, layout)
		if err != nil {
			t.Fatalf("inspect: %v", err)
		}
		inspectDur = time.Since(start)
	}
	t.Logf("phase db.Inspect: %s", inspectDur)

	var ensureDur time.Duration
	{
		start := time.Now()
		if err := audit.EnsureTables(ctx, tc); err != nil {
			t.Fatalf("ensure audit tables: %v", err)
		}
		ensureDur = time.Since(start)
	}
	t.Logf("phase audit.EnsureTables: %s", ensureDur)

	var checksumsDur time.Duration
	var checksums map[string][32]byte
	{
		start := time.Now()
		checksums, err = audit.LoadChecksums(ctx, tc, layout.NormalizedKeys())
		if err != nil {
			t.Fatalf("load checksums: %v", err)
		}
		checksumsDur = time.Since(start)
	}
	t.Logf("phase audit.LoadChecksums: %s", checksumsDur)

	var diffDur time.Duration
	var plan *types.MigrationPlan
	{
		start := time.Now()
		computer := diff.NewComputer(cfg)
		plan, err = computer.Compute(ctx, layout, state, checksums)
		if err != nil {
			t.Fatalf("compute: %v", err)
		}
		diffDur = time.Since(start)
	}
	t.Logf("phase diff.Compute (Go-heavy; typically little time in driver.Conn): %s", diffDur)

	pipelineWall := connectDur + scanDur + inspectDur + ensureDur + checksumsDur + diffDur
	t.Logf("phase SUM wall (connect+scan+inspect+ensure+checksums+diff): %s", pipelineWall)

	tc.logSummary(t)

	t.Logf("plan summary: objects=%d blocked=%v schemas=%d",
		len(plan.Objects), plan.Blocked, len(plan.Schemas))
	if countByAction(plan, types.ActionCreateObject) == 0 {
		t.Error("expected some objects to be created in empty DB")
	}
}

// TestIntegration_PhaseReport_ApplyPipeline adds apply + post-inspect timings
// (same shape as TestIntegration_Apply_AllObjects, with instrumentation).
func TestIntegration_PhaseReport_ApplyPipeline(t *testing.T) {
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1")
	}

	cfg := configFromEnv()
	sqlRoot := filepath.Join("..", "..", ".temp", "sql")
	cfg.SQLRoot = sqlRoot
	ctx := context.Background()

	t0 := time.Now()
	ensureTestDatabase(t, ctx)
	t.Logf("phase ensureTestDatabase: %s", time.Since(t0))

	raw, err := mssql.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	if _, err := raw.ExecContext(ctx, `
		IF SCHEMA_ID('smoke') IS NULL EXEC('CREATE SCHEMA smoke')
	`, nil); err != nil {
		_ = raw.Close()
		t.Fatalf("create schema: %v", err)
	}

	tc := newTimingConn(raw)
	defer func() { _ = tc.Close() }()

	var scanDur time.Duration
	var layout fs.Layout
	{
		st := time.Now()
		scanner := fs.NewScanner()
		layout, err = scanner.Scan(ctx, sqlRoot)
		if err != nil {
			t.Fatalf("scan: %v", err)
		}
		scanDur = time.Since(st)
	}
	t.Logf("phase fs.Scan: %s", scanDur)

	var inspect1 time.Duration
	var state *db.State
	{
		st := time.Now()
		inspector := db.NewInspector()
		state, err = inspector.Inspect(ctx, tc, layout)
		if err != nil {
			t.Fatalf("inspect: %v", err)
		}
		inspect1 = time.Since(st)
	}
	t.Logf("phase db.Inspect (pre-apply): %s", inspect1)

	if err := audit.EnsureTables(ctx, tc); err != nil {
		t.Fatalf("ensure audit tables: %v", err)
	}
	checksums, err := audit.LoadChecksums(ctx, tc, layout.NormalizedKeys())
	if err != nil {
		t.Fatalf("load checksums: %v", err)
	}

	computer := diff.NewComputer(cfg)
	plan, err := computer.Compute(ctx, layout, state, checksums)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}

	b := bus.New()
	auditSub := audit.NewSubscriber(b, tc)
	_ = auditSub
	b.Publish(ctx, types.EventRunStarted, &types.RunStarted{Command: "migrate"})
	b.Publish(ctx, types.EventDiffComputed, &types.DiffResult{Plan: plan})

	var applyDur time.Duration
	var result *apply.ApplyResult
	{
		st := time.Now()
		executor := apply.New()
		result, err = executor.Execute(ctx, tc, *plan, layout, b)
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		applyDur = time.Since(st)
	}
	t.Logf("phase apply.Execute: %s", applyDur)

	b.Publish(ctx, types.EventRunFinished, &types.RunFinished{Command: "migrate", Result: "success", ExitCode: 0})

	var inspect2 time.Duration
	{
		st := time.Now()
		inspector2 := db.NewInspector()
		_, err = inspector2.Inspect(ctx, tc, layout)
		if err != nil {
			t.Fatalf("post-apply inspect: %v", err)
		}
		inspect2 = time.Since(st)
	}
	t.Logf("phase db.Inspect (post-apply): %s", inspect2)

	t.Logf("apply result: applied=%d failed=%d skipped=%d", result.Applied, result.Failed, result.Skipped)
	tc.logSummary(t)

	if result.Applied == 0 {
		t.Error("expected to apply at least some objects")
	}
}
