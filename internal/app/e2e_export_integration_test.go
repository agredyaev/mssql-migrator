//go:build integration

package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"reporting-db-migrations/internal/driver/mssql"
	"reporting-db-migrations/internal/prodgate"
)

// TestE2E_ExportPlanSnapshot runs the plan pipeline and writes a comparable snapshot JSON.
// Used by ops/perf/go_rust_e2e.sh for Go↔Rust parity on .temp/sql.
//
// Env:
//   - RMIG_RUN_SQLSERVER_INTEGRATION=1 (required)
//   - RMIG_E2E_EXPORT_SNAPSHOT — output path (required)
//   - RMIG_GATE_SKIP_DB_RESET=1 — skip DROP/CREATE (script sets this on second run)
func TestE2E_ExportPlanSnapshot(t *testing.T) {
	if os.Getenv("RMIG_RUN_SQLSERVER_INTEGRATION") != "1" {
		t.Skip("set RMIG_RUN_SQLSERVER_INTEGRATION=1")
	}
	outPath := os.Getenv("RMIG_E2E_EXPORT_SNAPSHOT")
	if outPath == "" {
		t.Skip("set RMIG_E2E_EXPORT_SNAPSHOT")
	}

	cfg := configFromEnv()
	sqlRoot := filepath.Join("..", "..", ".temp", "sql")
	cfg.SQLRoot = sqlRoot

	ctx := context.Background()
	if os.Getenv("RMIG_GATE_SKIP_DB_RESET") == "1" || testDBResetMode() == "never" {
		t.Log("ensureTestDatabase: skipped")
	} else {
		ensureTestDatabase(t, ctx)
	}

	raw, err := mssql.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	tc := newTimingConn(raw)
	defer func() { _ = tc.Close() }()

	plan, _, timings, err := RunPlanPipeline(ctx, cfg, tc, sqlRoot, PlanPipelineOptions{EnsureAudit: true})
	if err != nil {
		t.Fatalf("plan pipeline: %v", err)
	}
	_ = timings
	snap := prodgate.SnapshotFromPlan(plan)
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := prodgate.WriteJSONFile(outPath, snap); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	t.Logf("exported snapshot (%d objects) to %s in %s", len(snap.Objects), outPath, time.Now().Format(time.RFC3339))
}
