//go:build integration

package app

import (
	"context"
	"encoding/json"
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
	"reporting-db-migrations/internal/prodgate"
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
	fetchTotal time.Duration
	execTotal  time.Duration
	pingTotal  time.Duration
	queryCalls int64
	fetchCalls int64
	execCalls  int64
	pingCalls  int64
}

type timingRows struct {
	inner driver.Rows
	tc    *timingConn
}

func newTimingConn(inner driver.Conn) *timingConn {
	return &timingConn{inner: inner}
}

func (c *timingConn) wrapRows(rows driver.Rows, err error) (driver.Rows, error) {
	if err != nil || rows == nil {
		return rows, err
	}
	return &timingRows{inner: rows, tc: c}, nil
}

func (c *timingConn) QueryContext(ctx context.Context, query string, args ...any) (driver.Rows, error) {
	start := time.Now()
	rows, err := c.inner.QueryContext(ctx, query, args...)
	c.addQuery(time.Since(start))
	return c.wrapRows(rows, err)
}

func (c *timingConn) QueryStringsContext(ctx context.Context, query string, args []string) (driver.Rows, error) {
	start := time.Now()
	rows, err := c.inner.QueryStringsContext(ctx, query, args)
	c.addQuery(time.Since(start))
	return c.wrapRows(rows, err)
}

func (c *timingConn) QueryStringSlicesContext(ctx context.Context, query string, args1, args2 []string) (driver.Rows, error) {
	start := time.Now()
	rows, err := c.inner.QueryStringSlicesContext(ctx, query, args1, args2)
	c.addQuery(time.Since(start))
	return c.wrapRows(rows, err)
}

func (r *timingRows) Next() bool {
	start := time.Now()
	ok := r.inner.Next()
	r.tc.addFetch(time.Since(start))
	return ok
}

func (r *timingRows) Scan(dest ...any) error {
	start := time.Now()
	err := r.inner.Scan(dest...)
	r.tc.addFetch(time.Since(start))
	return err
}

func (r *timingRows) Err() error   { return r.inner.Err() }
func (r *timingRows) Close() error { return r.inner.Close() }

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

func (c *timingConn) addFetch(d time.Duration) {
	c.mu.Lock()
	c.fetchTotal += d
	c.fetchCalls++
	c.mu.Unlock()
}

func (c *timingConn) fetchMS() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return prodgate.DurMS(c.fetchTotal)
}

func (c *timingConn) logSummary(t *testing.T) {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	dbBoundary := c.queryTotal + c.fetchTotal + c.execTotal + c.pingTotal
	t.Logf("driver.Conn wall (Query return + Rows.Next/Scan + Exec + Ping): %s (Query=%s n=%d, Fetch=%s n=%d, Exec=%s n=%d, Ping=%s n=%d)",
		dbBoundary, c.queryTotal, c.queryCalls, c.fetchTotal, c.fetchCalls, c.execTotal, c.execCalls, c.pingTotal, c.pingCalls)
}

func maybeEnsureTestDatabase(t *testing.T, ctx context.Context) {
	t.Helper()
	if os.Getenv("RMIG_PHASE_SKIP_DB_RESET") == "1" {
		t.Log("phase ensureTestDatabase: skipped (RMIG_PHASE_SKIP_DB_RESET=1)")
		return
	}
	ensureTestDatabase(t, ctx)
}

// TestIntegration_PhaseReport_PlanPipeline prints per-phase wall times using the
// shared RunPlanPipeline helper (parallel inspect ‖ checksums, matches engine.runPlan).
func TestIntegration_PhaseReport_PlanPipeline(t *testing.T) {
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1")
	}

	cfg := configFromEnv()
	sqlRoot := filepath.Join("..", "..", ".temp", "sql")
	cfg.SQLRoot = sqlRoot
	cfg.SkipGit = true

	ctx := context.Background()

	t0 := time.Now()
	ensureTestDatabase(t, ctx)
	t.Logf("phase ensureTestDatabase: %s", time.Since(t0))

	startConn := time.Now()
	raw, err := mssql.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	timings := prodgate.PhaseTimings{ConnectMS: prodgate.DurMS(time.Since(startConn))}

	tc := newTimingConn(raw)
	defer func() { _ = tc.Close() }()

	plan, _, pipeTimings, err := RunPlanPipeline(ctx, cfg, tc, sqlRoot, PlanPipelineOptions{EnsureAudit: true})
	if err != nil {
		t.Fatalf("plan pipeline: %v", err)
	}
	timings.ScanMS = pipeTimings.ScanMS
	timings.InspectMS = pipeTimings.InspectMS
	timings.ChecksumsMS = pipeTimings.ChecksumsMS
	timings.EnsureMS = pipeTimings.EnsureMS
	timings.ParallelWallMS = pipeTimings.ParallelWallMS
	timings.AuditMS = pipeTimings.AuditMS
	timings.DiffMS = pipeTimings.DiffMS
	timings.PlanWallMS = pipeTimings.PlanWallMS

	logPhaseTimings(t, timings)
	tc.logSummary(t)

	if countByAction(plan, types.ActionCreateObject) == 0 {
		t.Error("expected some objects to be created in empty DB")
	}
}

// TestIntegration_PhaseReport_CLI_Plan runs full app.Run via runWithLookup (prod-like).
// Run: make test-int-phase-cli
func TestIntegration_PhaseReport_CLI_Plan(t *testing.T) {
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1")
	}

	cfg := configFromEnv()
	sqlRoot := filepath.Join("..", "..", ".temp", "sql")
	reportDir := t.TempDir()
	ctx := context.Background()

	t0 := time.Now()
	maybeEnsureTestDatabase(t, ctx)
	if os.Getenv("RMIG_PHASE_SKIP_DB_RESET") != "1" {
		t.Logf("phase ensureTestDatabase: %s (excluded from cli_wall_ms)", time.Since(t0))
	}

	timings := &prodgate.PhaseTimings{}
	enableIntegrationPhaseTrace(timings)
	defer disableIntegrationPhaseTrace()

	var tc *timingConn
	connector := phaseTraceConnector(t, timings, &tc)
	lookup := integrationPhaseLookup(cfg, sqlRoot, reportDir)

	cliStart := time.Now()
	code := runWithLookup([]string{"rmig", "--env", "/nonexistent/.env", "plan"}, lookup, connector)
	timings.CLIWallMS = prodgate.DurMS(time.Since(cliStart))

	if code != 0 {
		t.Fatalf("rmig plan exit code %d", code)
	}
	if tc != nil {
		timings.FetchMS = tc.fetchMS()
		tc.logSummary(t)
	}
	logPhaseTimings(t, *timings)
	writePhaseTimingsReport(t, *timings)

	if _, err := os.Stat(filepath.Join(reportDir, ".plan.json")); err != nil {
		t.Errorf(".plan.json: %v", err)
	}
}

// TestIntegration_PhaseReport_CLI_Migrate runs full migrate via runWithLookup.
func TestIntegration_PhaseReport_CLI_Migrate(t *testing.T) {
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1")
	}

	cfg := configFromEnv()
	sqlRoot := filepath.Join("..", "..", ".temp", "sql")
	reportDir := t.TempDir()
	ctx := context.Background()

	t0 := time.Now()
	maybeEnsureTestDatabase(t, ctx)
	if os.Getenv("RMIG_PHASE_SKIP_DB_RESET") != "1" {
		t.Logf("phase ensureTestDatabase: %s", time.Since(t0))
	}

	timings := &prodgate.PhaseTimings{}
	enableIntegrationPhaseTrace(timings)
	defer disableIntegrationPhaseTrace()

	var tc *timingConn
	connector := phaseTraceConnector(t, timings, &tc)
	lookup := integrationPhaseLookup(cfg, sqlRoot, reportDir)

	cliStart := time.Now()
	code := runWithLookup([]string{"rmig", "--env", "/nonexistent/.env", "migrate"}, lookup, connector)
	timings.CLIWallMS = prodgate.DurMS(time.Since(cliStart))

	if code != 0 {
		t.Fatalf("rmig migrate exit code %d", code)
	}
	if tc != nil {
		timings.FetchMS = tc.fetchMS()
		tc.logSummary(t)
	}
	logPhaseTimings(t, *timings)
	writePhaseTimingsReport(t, *timings)

	if timings.ApplyMS == 0 {
		t.Log("warning: apply_ms is 0 (no apply work or observer not fired)")
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

func integrationPhaseLookup(cfg types.Config, sqlRoot, reportDir string) envLookupFn {
	return func(key string) (string, bool) {
		switch key {
		case "RM_DB_SERVER":
			return cfg.Server, true
		case "RM_DB_PORT":
			return cfg.Port, true
		case "RM_DB_DATABASE":
			return cfg.Database, true
		case "RM_DB_AUTH":
			return cfg.DBAuth, true
		case "RM_DB_USER":
			return cfg.User, true
		case "RM_DB_PASSWORD":
			return cfg.Password, true
		case "RM_DB_ENCRYPT":
			return "false", true
		case "RM_DB_TRUST_SERVER_CERTIFICATE":
			return "true", true
		case "RM_SQL_ROOT":
			return sqlRoot, true
		case "RM_SQL_BASE":
			return sqlRoot, true
		case "RM_REPORT_DIR":
			return reportDir, true
		case "RM_SKIP_GIT":
			return "1", true
		case "RM_LOG_LEVEL":
			return cfg.LogLevel, true
		default:
			return "", false
		}
	}
}

func phaseTraceConnector(t *testing.T, timings *prodgate.PhaseTimings, tcOut **timingConn) Connector {
	t.Helper()
	return func(ctx context.Context, cfg types.Config) (driver.Conn, error) {
		start := time.Now()
		raw, err := mssql.Open(ctx, cfg)
		if err != nil {
			return nil, err
		}
		timings.ConnectMS = prodgate.DurMS(time.Since(start))
		tc := newTimingConn(raw)
		*tcOut = tc
		return tc, nil
	}
}

func logPhaseTimings(t *testing.T, timings prodgate.PhaseTimings) {
	t.Helper()
	data, err := json.MarshalIndent(timings, "", "  ")
	if err != nil {
		t.Fatalf("marshal timings: %v", err)
	}
	t.Logf("phase timings JSON:\n%s", string(data))
}

func writePhaseTimingsReport(t *testing.T, timings prodgate.PhaseTimings) {
	t.Helper()
	path := os.Getenv("RMIG_CLI_PHASE_REPORT")
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir phase report dir: %v", err)
	}
	data, err := json.MarshalIndent(timings, "", "  ")
	if err != nil {
		t.Fatalf("marshal phase report: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write phase report %s: %v", path, err)
	}
	t.Logf("wrote phase timings to %s", path)
}
