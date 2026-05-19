//go:build integration

package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reporting-db-migrations/internal/driver/mssql"
	"reporting-db-migrations/internal/prodgate"
)

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
//   - RMIG_GATE_CHANGED_FILES — comma-separated paths (test override; prod uses auto git delta)
//   - RMIG_GATE_GIT_BASE — optional git base ref (test override)
//   - RMIG_INSPECT_FULL=1 — force full catalog inspect
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

	plan, layout, timings, err := RunPlanPipeline(ctx, cfg, tc, sqlRoot, PlanPipelineOptions{EnsureAudit: true})
	if err != nil {
		t.Fatalf("plan pipeline: %v", err)
	}
	timings.ConnectMS = connectMS

	current := prodgate.SnapshotFromPlan(plan)
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

	pathsResult, err := prodgate.ResolveChangedPaths(sqlRoot)
	if err != nil {
		t.Fatalf("resolve changed paths: %v", err)
	}
	changedPaths := pathsResult.Paths
	deltaKeys := prodgate.KeysForChangedPaths(layout, changedPaths)
	deltaKeys = prodgate.ExpandDeltaClosure(layout, deltaKeys)
	t.Logf("delta source: %s (full_inspect=%v)", pathsResult.Source, pathsResult.FullInspect)
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
		Timings:          timings,
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
	t.Logf("phase timings (plan pipeline): connect=%dms scan=%dms inspect=%dms checksums=%dms ensure=%dms parallel_wall=%dms audit=%dms diff=%dms plan_wall=%dms",
		timings.ConnectMS, timings.ScanMS, timings.InspectMS, timings.ChecksumsMS, timings.EnsureMS,
		timings.ParallelWallMS, timings.AuditMS, timings.DiffMS, timings.PlanWallMS)

	if !result.Go {
		for _, msg := range result.Messages {
			t.Errorf("no-go: %s", msg)
		}
		t.Fatalf("prod gate: NO-GO")
	}
	t.Log("prod gate: GO")
}
