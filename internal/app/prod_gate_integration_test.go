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

	"reporting-db-migrations/internal/audit"
	"reporting-db-migrations/internal/db"
	"reporting-db-migrations/internal/diff"
	"reporting-db-migrations/internal/driver/mssql"
	"reporting-db-migrations/internal/fs"
	"reporting-db-migrations/internal/prodgate"
	"reporting-db-migrations/internal/types"
)

type planPipelineResult struct {
	Plan    *types.MigrationPlan
	Layout  fs.Layout
	Timings prodgate.PhaseTimings
}

func runPlanPipelineForGate(t *testing.T, ctx context.Context, cfg types.Config, sqlRoot string, tc *timingConn) planPipelineResult {
	t.Helper()
	var timings prodgate.PhaseTimings
	startAll := time.Now()

	start := time.Now()
	scanner := fs.NewScanner()
	layout, err := scanner.Scan(ctx, sqlRoot)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	timings.ScanMS = prodgate.DurMS(time.Since(start))

	start = time.Now()
	if err := audit.EnsureTables(ctx, tc); err != nil {
		t.Fatalf("ensure audit tables: %v", err)
	}
	ensureDur := time.Since(start)

	keys := layout.NormalizedKeys()
	var (
		state      *db.State
		checksums  map[string][32]byte
		inspErr    error
		loadErr    error
		inspectDur time.Duration
		loadDur    time.Duration
	)
	start = time.Now()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		t0 := time.Now()
		inspector := db.NewInspector()
		state, inspErr = inspector.Inspect(ctx, tc, layout)
		inspectDur = time.Since(t0)
	}()
	go func() {
		defer wg.Done()
		t0 := time.Now()
		var err error
		checksums, err = audit.LoadChecksums(ctx, tc, keys)
		loadErr = err
		loadDur = time.Since(t0)
	}()
	wg.Wait()
	_ = time.Since(start) // parallel wall; plan_wall uses startAll
	if inspErr != nil {
		t.Fatalf("inspect: %v", inspErr)
	}
	if loadErr != nil {
		t.Fatalf("load checksums: %v", loadErr)
	}
	timings.InspectMS = prodgate.DurMS(inspectDur)
	timings.AuditMS = prodgate.DurMS(ensureDur + loadDur)

	start = time.Now()
	computer := diff.NewComputer(cfg)
	plan, err := computer.Compute(ctx, layout, state, checksums)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	timings.DiffMS = prodgate.DurMS(time.Since(start))
	timings.PlanWallMS = prodgate.DurMS(time.Since(startAll))

	return planPipelineResult{Plan: plan, Layout: layout, Timings: timings}
}

func prodGateBaselinePath() string {
	return filepath.Join("testdata", "prod_gate", "plan_baseline_empty_db.json")
}

// TestProdGate_IncrementalPlan runs the prod go/no-go gate: compare current plan
// to a committed baseline, evaluating only keys in the git/env delta when set.
//
// Env:
//   - RMIG_RUN_SQLSERVER_INTEGRATION=1 (required)
//   - RMIG_GATE_SKIP_DB_RESET=1 — do not drop/recreate DB (closer to prod)
//   - RMIG_GATE_UPDATE_BASELINE=1 — rewrite testdata baseline (maintainers only)
//   - RMIG_GATE_CHANGED_FILES — comma-separated paths limiting allowed diffs
//   - RMIG_GATE_GIT_BASE — e.g. origin/main; used when CHANGED_FILES unset
//   - RMIG_GATE_MAX_PLAN_WALL_MS — optional plan-phase wall SLO
//   - RMIG_GATE_REPORT — write GateResult JSON to this path
func TestProdGate_IncrementalPlan(t *testing.T) {
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1")
	}

	cfg := configFromEnv()
	sqlRoot := filepath.Join("..", "..", ".temp", "sql")
	cfg.SQLRoot = sqlRoot

	ctx := context.Background()

	if os.Getenv("RMIG_GATE_SKIP_DB_RESET") != "1" {
		t0 := time.Now()
		ensureTestDatabase(t, ctx)
		t.Logf("phase ensureTestDatabase: %s (excluded from plan SLO)", time.Since(t0))
	} else {
		t.Log("phase ensureTestDatabase: skipped (RMIG_GATE_SKIP_DB_RESET=1)")
	}

	startConn := time.Now()
	raw, err := mssql.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	connectMS := prodgate.DurMS(time.Since(startConn))

	tc := newTimingConn(raw)
	defer func() { _ = tc.Close() }()

	pipe := runPlanPipelineForGate(t, ctx, cfg, sqlRoot, tc)
	pipe.Timings.ConnectMS = connectMS

	current := prodgate.SnapshotFromPlan(pipe.Plan)
	baselinePath := prodGateBaselinePath()

	if os.Getenv("RMIG_GATE_UPDATE_BASELINE") == "1" {
		if err := os.MkdirAll(filepath.Dir(baselinePath), 0755); err != nil {
			t.Fatalf("mkdir baseline: %v", err)
		}
		if err := prodgate.WriteJSONFile(baselinePath, current); err != nil {
			t.Fatalf("write baseline: %v", err)
		}
		t.Logf("updated baseline at %s", baselinePath)
	}

	baseline, err := prodgate.ReadJSONFile(baselinePath)
	if err != nil {
		t.Fatalf("read baseline %s: %v (run with RMIG_GATE_UPDATE_BASELINE=1 to create)", baselinePath, err)
	}

	changedPaths := prodgate.ChangedPathsFromEnv()
	if len(changedPaths) == 0 {
		if gitBase := os.Getenv("RMIG_GATE_GIT_BASE"); gitBase != "" {
			repoRoot := filepath.Join("..", "..")
			changedPaths, err = prodgate.ChangedPathsFromGit(repoRoot, gitBase)
			if err != nil {
				t.Fatalf("git delta paths: %v", err)
			}
		}
	}
	deltaKeys := prodgate.KeysForChangedPaths(pipe.Layout, changedPaths)
	if len(changedPaths) > 0 {
		t.Logf("delta: %d changed path(s) -> %d object key(s)", len(changedPaths), len(deltaKeys))
		for _, p := range changedPaths {
			t.Logf("  changed path: %s", p)
		}
	} else {
		t.Log("delta: empty (strict full-plan match against baseline)")
	}

	result := prodgate.Evaluate(prodgate.GateInput{
		Baseline:         baseline,
		Current:          current,
		DeltaKeys:        deltaKeys,
		StrictUnexpected: true,
		Timings:          pipe.Timings,
		MaxPlanWallMS:    prodgate.MaxPlanWallMSFromEnv(),
	})

	report, _ := json.MarshalIndent(result, "", "  ")
	t.Logf("prod gate result:\n%s", string(report))

	if path := os.Getenv("RMIG_GATE_REPORT"); path != "" {
		if err := os.WriteFile(path, report, 0644); err != nil {
			t.Fatalf("write report: %v", err)
		}
		t.Logf("wrote gate report to %s", path)
	}

	tc.logSummary(t)
	t.Logf("phase timings (plan pipeline): connect=%dms scan=%dms inspect=%dms audit=%dms diff=%dms plan_wall=%dms",
		pipe.Timings.ConnectMS, pipe.Timings.ScanMS, pipe.Timings.InspectMS,
		pipe.Timings.AuditMS, pipe.Timings.DiffMS, pipe.Timings.PlanWallMS)

	if !result.Go {
		for _, msg := range result.Messages {
			t.Errorf("no-go: %s", msg)
		}
		t.Fatalf("prod gate: NO-GO")
	}
	t.Log("prod gate: GO")
}
