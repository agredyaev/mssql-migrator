#!/usr/bin/env bash
# Go↔Rust scenario e2e: behavior (action counts) + phase timings on .temp/sql.
# Pipeline path: Go RunPlanPipeline ↔ Rust run_plan_pipeline (direct SQL connect).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
ARTIFACTS="$ROOT/ops/perf/artifacts"
mkdir -p "$ARTIFACTS"

GO_EMPTY="${RMIG_E2E_GO_EMPTY:-$ARTIFACTS/go_e2e_empty_db_plan.json}"
RUST_EMPTY="${RMIG_E2E_RUST_EMPTY:-$ARTIFACTS/rust_e2e_empty_db_plan.json}"
GO_WARM="${RMIG_E2E_GO_WARM:-$ARTIFACTS/go_e2e_warm_db_plan.json}"
RUST_WARM="${RMIG_E2E_RUST_WARM:-$ARTIFACTS/rust_e2e_warm_db_plan.json}"
REPORT="${RMIG_E2E_REPORT:-$ARTIFACTS/go_rust_e2e_report.txt}"

export RMIG_RUN_SQLSERVER_INTEGRATION=1
export RM_SKIP_GIT=1
export RM_DB_SERVER="${RM_DB_SERVER:-localhost}"
export RM_DB_PORT="${RM_DB_PORT:-1433}"
export RM_DB_DATABASE="${RM_DB_DATABASE:-rmig_test}"
export RM_DB_USER="${RM_DB_USER:-sa}"
export RM_DB_PASSWORD="${RM_DB_PASSWORD:-yourStrong(!)Password}"
export RM_DB_ENCRYPT="${RM_DB_ENCRYPT:-false}"
export RM_DB_TRUST_SERVER_CERTIFICATE="${RM_DB_TRUST_SERVER_CERTIFICATE:-true}"
export RM_SQL_ROOT="${RM_SQL_ROOT:-$ROOT/.temp/sql}"
export RM_SQL_BASE="${RM_SQL_BASE:-$ROOT/.temp/sql}"
export RMIG_E2E_RUN_ID="${RMIG_E2E_RUN_ID:-$(date +%s)}"
export RMIG_E2E_TIMING_FACTOR="${RMIG_E2E_TIMING_FACTOR:-3.0}"
export RMIG_E2E_TIMING_SLACK_MS="${RMIG_E2E_TIMING_SLACK_MS:-100}"
export RMIG_CATALOG_CACHE="${RMIG_CATALOG_CACHE:-0}"

run_rust_scenario() {
  local scenario="$1"
  local go_report="$2"
  local rust_report="$3"
  export RMIG_E2E_SCENARIO="$scenario"
  export RMIG_E2E_GO_REPORT="$go_report"
  export RMIG_E2E_RUST_REPORT="$rust_report"
  export RMIG_GATE_SKIP_DB_RESET=1
  (cd "$ROOT/rust" && cargo test --release -p migrator-core --test go_rust_scenario_integration go_rust_scenario_matches_go_reference -- --nocapture --test-threads=1)
}

echo "== go-rust e2e: scenarios on .temp/sql (pipeline path) =="

echo "== scenario empty_db_plan: Go export (reset DB via test) =="
export RMIG_E2E_SCENARIO=empty_db_plan
export RMIG_E2E_EXPORT_REPORT="$GO_EMPTY"
unset RMIG_GATE_SKIP_DB_RESET
go test -tags=integration ./internal/app/ -run TestE2E_ExportScenarioReport -v -count=1

echo "== scenario empty_db_plan: Rust vs Go =="
run_rust_scenario empty_db_plan "$GO_EMPTY" "$RUST_EMPTY"

echo "== apply smoke baseline (Go) =="
export RMIG_GATE_SKIP_DB_RESET=1
go test -tags=integration ./internal/app/ -run TestE2E_ApplySmokeBaseline -v -count=1

echo "== scenario warm_db_plan: Go export =="
export RMIG_E2E_SCENARIO=warm_db_plan
export RMIG_E2E_EXPORT_REPORT="$GO_WARM"
go test -tags=integration ./internal/app/ -run TestE2E_ExportScenarioReport -v -count=1

echo "== scenario warm_db_plan: Rust vs Go =="
run_rust_scenario warm_db_plan "$GO_WARM" "$RUST_WARM"

{
  echo "go-rust e2e scenarios: PASS"
  echo "empty_db_plan: $GO_EMPTY ↔ $RUST_EMPTY"
  echo "warm_db_plan:  $GO_WARM ↔ $RUST_WARM"
  echo "timing factor=${RMIG_E2E_TIMING_FACTOR} slack_ms=${RMIG_E2E_TIMING_SLACK_MS}"
} | tee "$REPORT"

echo "Report: $REPORT"
