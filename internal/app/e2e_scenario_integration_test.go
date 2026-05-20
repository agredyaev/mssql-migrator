//go:build integration

package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reporting-db-migrations/internal/apply"
	"reporting-db-migrations/internal/audit"
	"reporting-db-migrations/internal/bus"
	"reporting-db-migrations/internal/driver/mssql"
	"reporting-db-migrations/internal/errors"
	"reporting-db-migrations/internal/prodgate"
	"reporting-db-migrations/internal/types"
)

// TestE2E_ExportScenarioReport runs RunPlanPipeline and exports E2EScenarioReport JSON.
//
// Env:
//   - RMIG_E2E_SCENARIO — plan scenario id (required)
//   - RMIG_E2E_EXPORT_REPORT — output path (required)
//   - RMIG_GATE_SKIP_DB_RESET=1 — skip DROP/CREATE (warm / skip_unchanged / catalog_cache)
func TestE2E_ExportScenarioReport(t *testing.T) {
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1")
	}
	scenario := os.Getenv("RMIG_E2E_SCENARIO")
	if scenario == "" {
		t.Skip("set RMIG_E2E_SCENARIO")
	}
	outPath := os.Getenv("RMIG_E2E_EXPORT_REPORT")
	if outPath == "" {
		t.Skip("set RMIG_E2E_EXPORT_REPORT")
	}

	cfg := configFromEnv()
	sqlRoot := filepath.Join("..", "..", ".temp", "sql")
	cfg.SQLRoot = sqlRoot
	cfg.SkipGit = true

	ctx := context.Background()
	skipReset := os.Getenv("RMIG_GATE_SKIP_DB_RESET") == "1" || testDBResetMode() == "never" ||
		scenario == "skip_unchanged_plan" || scenario == "warm_db_plan" || scenario == "catalog_cache_plan"
	if skipReset {
		t.Log("ensureTestDatabase: skipped")
	} else {
		resetTestDatabase(t, ctx)
	}

	raw, err := mssql.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	tc := newTimingConn(raw)
	defer func() { _ = tc.Close() }()

	if err := audit.EnsureTables(ctx, tc); err != nil {
		t.Fatalf("ensure audit tables: %v", err)
	}

	plan, _, timings, err := RunPlanPipeline(ctx, cfg, tc, sqlRoot, PlanPipelineOptions{EnsureAudit: true})
	if err != nil {
		t.Fatalf("plan pipeline: %v", err)
	}

	io := prodgate.DbIoProfile{
		ConnectMS:  timings.ConnectMS,
		QueryMS:    prodgate.DurMS(tc.queryTotal),
		QueryCalls: tc.queryCalls,
		FetchMS:    prodgate.DurMS(tc.fetchTotal),
		FetchCalls: tc.fetchCalls,
		ExecMS:     prodgate.DurMS(tc.execTotal),
		ExecCalls:  tc.execCalls,
	}
	rep := prodgate.BuildE2EScenarioReport(scenario, plan, timings, io)
	switch scenario {
	case "empty_db_plan":
		rep.SetupSteps = []string{"reset_db", "plan_pipeline"}
	case "warm_db_plan":
		rep.SetupSteps = []string{"apply_baseline", "plan_pipeline"}
	case "skip_unchanged_plan":
		rep.SetupSteps = []string{"apply_baseline", "plan_pipeline_after_audit"}
	case "catalog_cache_plan":
		rep.SetupSteps = []string{"apply_baseline", "plan_pipeline", "catalog_cache=1"}
	}
	if err := prodgate.WriteE2EReportFile(outPath, rep); err != nil {
		t.Fatalf("write report: %v", err)
	}
	t.Logf("scenario %s: objects=%d actions=%v plan_wall=%dms diff=%dms inspect=%dms",
		scenario, len(plan.Objects), rep.ActionCounts, timings.PlanWallMS, timings.DiffMS, timings.InspectMS)
}

// TestE2E_ExportApplyReport exports apply outcome for apply_smoke_result scenario.
func TestE2E_ExportApplyReport(t *testing.T) {
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1")
	}
	scenario := os.Getenv("RMIG_E2E_SCENARIO")
	if scenario != "apply_smoke_result" {
		t.Skip("set RMIG_E2E_SCENARIO=apply_smoke_result")
	}
	outPath := os.Getenv("RMIG_E2E_EXPORT_REPORT")
	if outPath == "" {
		t.Skip("set RMIG_E2E_EXPORT_REPORT")
	}

	cfg := configFromEnv()
	sqlRoot := filepath.Join("..", "..", ".temp", "sql")
	cfg.SQLRoot = sqlRoot
	cfg.SkipGit = true

	ctx := context.Background()
	raw, err := mssql.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	tc := newTimingConn(raw)
	defer func() { _ = tc.Close() }()

	if err := audit.EnsureTables(ctx, tc); err != nil {
		t.Fatalf("ensure audit tables: %v", err)
	}

	applied, failed, skipped, applyErrs := runApplySmoke(t, ctx, tc, cfg, sqlRoot)
	auditRows := countAuditObjectRows(t, ctx, tc)

	rep := prodgate.BuildE2EApplyReport(scenario, applied, failed, skipped, auditRows, applyErrs)
	rep.SetupSteps = []string{"plan_pipeline", "apply_execute"}
	if err := prodgate.WriteE2EApplyReportFile(outPath, rep); err != nil {
		t.Fatalf("write apply report: %v", err)
	}
	t.Logf("apply_smoke: applied=%d failed=%d skipped=%d audit_rows=%d", applied, failed, skipped, auditRows)
}

// TestE2E_ExportGateReport exports prod gate result for prod_gate_cold scenario.
func TestE2E_ExportGateReport(t *testing.T) {
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1")
	}
	scenario := os.Getenv("RMIG_E2E_SCENARIO")
	if scenario != "prod_gate_cold" {
		t.Skip("set RMIG_E2E_SCENARIO=prod_gate_cold")
	}
	outPath := os.Getenv("RMIG_E2E_EXPORT_REPORT")
	if outPath == "" {
		t.Skip("set RMIG_E2E_EXPORT_REPORT")
	}

	cfg := configFromEnv()
	sqlRoot := filepath.Join("..", "..", ".temp", "sql")
	cfg.SQLRoot = sqlRoot
	cfg.SkipGit = true

	ctx := context.Background()
	if os.Getenv("RMIG_GATE_SKIP_DB_RESET") == "1" || testDBResetMode() == "never" {
		t.Log("ensureTestDatabase: skipped")
	} else {
		resetTestDatabase(t, ctx)
	}

	raw, err := mssql.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	tc := newTimingConn(raw)
	defer func() { _ = tc.Close() }()

	plan, layout, timings, err := RunPlanPipeline(ctx, cfg, tc, sqlRoot, PlanPipelineOptions{EnsureAudit: true})
	if err != nil {
		t.Fatalf("plan pipeline: %v", err)
	}

	current := prodgate.SnapshotFromPlan(plan)
	baselinePath := prodGateBaselinePath()
	baseline, err := prodgate.ReadJSONFile(baselinePath)
	if err != nil {
		t.Fatalf("read baseline %s: %v", baselinePath, err)
	}

	pathsResult, err := prodgate.ResolveChangedPaths(sqlRoot)
	if err != nil {
		t.Fatalf("resolve changed paths: %v", err)
	}
	deltaKeys := prodgate.KeysForChangedPaths(layout, pathsResult.Paths)
	deltaKeys = prodgate.ExpandDeltaClosure(layout, deltaKeys)

	result := prodgate.Evaluate(prodgate.GateInput{
		Baseline:         baseline,
		Current:          current,
		DeltaKeys:        deltaKeys,
		StrictUnexpected: true,
		Timings:          timings,
		MaxPlanWallMS:    prodgate.MaxPlanWallMSFromEnv(),
	})

	rep := prodgate.BuildE2EGateReport(scenario, current, result)
	rep.SetupSteps = []string{"reset_db", "plan_pipeline", "gate_evaluate"}
	if err := prodgate.WriteE2EGateReportFile(outPath, rep); err != nil {
		t.Fatalf("write gate report: %v", err)
	}
	t.Logf("prod_gate_cold: go=%v messages=%d", rep.GateGo, len(rep.Messages))
}

// TestE2E_ExportBlockedMigrate exports blocked migrate outcome for blocked_table_plan.
func TestE2E_ExportBlockedMigrate(t *testing.T) {
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1")
	}
	scenario := os.Getenv("RMIG_E2E_SCENARIO")
	if scenario != "blocked_table_plan" {
		t.Skip("set RMIG_E2E_SCENARIO=blocked_table_plan")
	}
	outPath := os.Getenv("RMIG_E2E_EXPORT_REPORT")
	if outPath == "" {
		t.Skip("set RMIG_E2E_EXPORT_REPORT")
	}

	cfg := configFromEnv()
	sqlRoot := filepath.Join("..", "..", ".temp", "sql")
	cfg.SQLRoot = sqlRoot
	cfg.SQLBase = sqlRoot
	cfg.SkipGit = false

	ctx := context.Background()
	raw, err := mssql.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	tc := newTimingConn(raw)
	defer func() { _ = tc.Close() }()

	if err := audit.EnsureTables(ctx, tc); err != nil {
		t.Fatalf("ensure audit tables: %v", err)
	}

	b := bus.New()
	_ = audit.NewSubscriber(b, tc)
	eng := newTestEngine(b, tc, cfg)
	if err := eng.Migrate(ctx); err != nil {
		t.Fatalf("baseline migrate: %v", err)
	}

	tempRepo := filepath.Join("..", "..", ".temp")
	tableSQLPath := filepath.Join(sqlRoot, "dactests", "smoke", "tables", "smoke_table.sql")
	cleanHead := gitHead(t, tempRepo)
	t.Cleanup(func() {
		cleanupScaffoldFiles(t, sqlRoot)
		gitCmd(t, tempRepo, "reset", "--hard", cleanHead)
	})

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
	gitCmd(t, tempRepo, "add", "sql/dactests/smoke/tables/smoke_table.sql")
	gitCmd(t, tempRepo, "commit", "-m", "test: e2e add added_at column")

	b2 := bus.New()
	_ = audit.NewSubscriber(b2, tc)
	eng2 := newTestEngine(b2, tc, cfg)
	exitCode := types.ExitOK
	blocked := false
	var blockers []string
	err = eng2.Migrate(ctx)
	if err != nil {
		exitCode = errors.ExitCode(err)
		blocked = exitCode == types.ExitPlanBlocked
	} else {
		t.Fatal("expected blocked migrate error")
	}

	scaffoldPaths := listScaffoldPaths(t, sqlRoot)
	rep := prodgate.E2EBlockedReport{
		Scenario:      scenario,
		SetupSteps:    []string{"baseline_migrate", "git_column_change", "migrate_blocked"},
		ExitCode:      exitCode,
		Blocked:       blocked,
		Blockers:      blockers,
		ScaffoldPaths: scaffoldPaths,
	}
	if err := prodgate.WriteE2EBlockedReportFile(outPath, rep); err != nil {
		t.Fatalf("write blocked report: %v", err)
	}
	t.Logf("blocked_table: exit=%d blocked=%v scaffolds=%d", exitCode, blocked, len(scaffoldPaths))
}

// TestE2E_ApplySmokeBaseline applies the empty-DB plan on .temp/sql (populates DB for warm_db_plan).
func TestE2E_ApplySmokeBaseline(t *testing.T) {
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1")
	}

	cfg := configFromEnv()
	sqlRoot := filepath.Join("..", "..", ".temp", "sql")
	cfg.SQLRoot = sqlRoot
	cfg.SkipGit = true

	ctx := context.Background()
	raw, err := mssql.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	tc := newTimingConn(raw)
	defer func() { _ = tc.Close() }()

	if err := audit.EnsureTables(ctx, tc); err != nil {
		t.Fatalf("ensure audit tables: %v", err)
	}

	_, _, _, _ = runApplySmoke(t, ctx, tc, cfg, sqlRoot)
}

func runApplySmoke(t *testing.T, ctx context.Context, tc *timingConn, cfg types.Config, sqlRoot string) (applied, failed, skipped int, errs []string) {
	t.Helper()
	plan, layout, _, err := RunPlanPipeline(ctx, cfg, tc, sqlRoot, PlanPipelineOptions{EnsureAudit: true})
	if err != nil {
		t.Fatalf("plan pipeline: %v", err)
	}
	if countByAction(plan, types.ActionCreateObject) == 0 && countByAction(plan, types.ActionAdoptExisting) == 0 {
		t.Fatal("expected create_object or adopt_existing actions before apply")
	}
	b := bus.New()
	_ = audit.NewSubscriber(b, tc)
	result, err := apply.New().Execute(ctx, tc, *plan, layout, b)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	return result.Applied, result.Failed, result.Skipped, result.Errors
}

func countAuditObjectRows(t *testing.T, ctx context.Context, tc *timingConn) int {
	t.Helper()
	rows, err := tc.QueryContext(ctx, "SELECT COUNT(*) FROM azdo_deploy_meta.history WHERE kind = 'object'", nil)
	if err != nil {
		t.Fatalf("audit count: %v", err)
	}
	defer rows.Close()
	var n int
	if rows.Next() {
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan audit count: %v", err)
		}
	}
	return n
}

func listScaffoldPaths(t *testing.T, sqlRoot string) []string {
	t.Helper()
	migrationDir := filepath.Join(sqlRoot, "dactests", "smoke", "tables", "_migrations", "smoke_table")
	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		return nil
	}
	var paths []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		paths = append(paths, filepath.Join(migrationDir, e.Name()))
	}
	return paths
}
