#!/usr/bin/env bash
# Full Go↔Rust e2e matrix on .temp/sql + Docker SQL Server.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
ARTIFACTS="$ROOT/ops/perf/artifacts"
mkdir -p "$ARTIFACTS"

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

REPORT="$ARTIFACTS/go_rust_e2e_all_report.txt"
: > "$REPORT"

reset_test_db() {
  echo "== reset rmig_test (orchestrator) =="
  docker compose exec -T mssql /opt/mssql-tools18/bin/sqlcmd \
    -S localhost -U sa -P "$RM_DB_PASSWORD" -C \
    -Q "IF DB_ID('${RM_DB_DATABASE}') IS NOT NULL BEGIN ALTER DATABASE [${RM_DB_DATABASE}] SET SINGLE_USER WITH ROLLBACK IMMEDIATE; DROP DATABASE [${RM_DB_DATABASE}]; END; CREATE DATABASE [${RM_DB_DATABASE}];"
}

run_plan_scenario() {
  local scenario="$1"
  local go_report="$2"
  local rust_report="$3"
  local skip_reset="${4:-1}"

  echo "== scenario ${scenario}: Go export =="
  export RMIG_E2E_SCENARIO="$scenario"
  export RMIG_E2E_EXPORT_REPORT="$go_report"
  if [[ "$skip_reset" == "0" ]]; then
    unset RMIG_GATE_SKIP_DB_RESET
  else
    export RMIG_GATE_SKIP_DB_RESET=1
  fi
  go test -tags=integration ./internal/app/ -run TestE2E_ExportScenarioReport -v -count=1 2>&1 | tail -5

  echo "== scenario ${scenario}: Rust vs Go =="
  export RMIG_E2E_GO_REPORT="$go_report"
  export RMIG_E2E_RUST_REPORT="$rust_report"
  export RMIG_GATE_SKIP_DB_RESET=1
  (cd "$ROOT/rust" && cargo test --release -p migrator-core --test go_rust_scenario_integration go_rust_scenario_matches_go_reference -- --nocapture --test-threads=1)
  echo "${scenario}: PASS" >> "$REPORT"
}

run_apply_go_export() {
  local go_report="$1"

  echo "== scenario apply_smoke_result: Go export =="
  export RMIG_E2E_SCENARIO=apply_smoke_result
  export RMIG_E2E_EXPORT_REPORT="$go_report"
  export RMIG_GATE_SKIP_DB_RESET=1
  go test -tags=integration ./internal/app/ -run TestE2E_ExportApplyReport -v -count=1 2>&1 | tail -5
}

run_apply_rust_compare() {
  local go_report="$1"
  local rust_report="$2"

  echo "== scenario apply_smoke_result: Rust vs Go (reset DB) =="
  export RMIG_E2E_SCENARIO=apply_smoke_result
  export RMIG_E2E_GO_REPORT="$go_report"
  export RMIG_E2E_RUST_REPORT="$rust_report"
  unset RMIG_GATE_SKIP_DB_RESET
  (cd "$ROOT/rust" && cargo test --release -p migrator-core --test go_rust_scenario_integration go_rust_scenario_matches_go_reference -- --nocapture --test-threads=1)
  echo "apply_smoke_result: PASS" >> "$REPORT"
}

run_gate_scenario() {
  local go_report="$1"
  local rust_report="$2"

  echo "== scenario prod_gate_cold: Go export =="
  export RMIG_E2E_SCENARIO=prod_gate_cold
  export RMIG_E2E_EXPORT_REPORT="$go_report"
  # Reuse cold empty DB after empty_db_plan (audit bootstrap done; no reset race).
  export RMIG_GATE_SKIP_DB_RESET=1
  go test -tags=integration ./internal/app/ -run TestE2E_ExportGateReport -v -count=1 2>&1 | tail -5

  echo "== scenario prod_gate_cold: Rust vs Go =="
  export RMIG_E2E_GO_REPORT="$go_report"
  export RMIG_E2E_RUST_REPORT="$rust_report"
  unset RMIG_GATE_SKIP_DB_RESET
  (cd "$ROOT/rust" && cargo test --release -p migrator-core --test go_rust_scenario_integration go_rust_scenario_matches_go_reference -- --nocapture --test-threads=1)
  echo "prod_gate_cold: PASS" >> "$REPORT"
}

run_blocked_scenario() {
  local go_report="$1"
  local rust_report="$2"

  reset_test_db

  echo "== scenario blocked_table_plan: Go export =="
  export RMIG_E2E_SCENARIO=blocked_table_plan
  export RMIG_E2E_EXPORT_REPORT="$go_report"
  export RMIG_GATE_SKIP_DB_RESET=1
  go test -tags=integration ./internal/app/ -run TestE2E_ExportBlockedMigrate -v -count=1 2>&1 | tail -20

  echo "== scenario blocked_table_plan: Rust vs Go =="
  export RMIG_E2E_GO_REPORT="$go_report"
  export RMIG_E2E_RUST_REPORT="$rust_report"
  (cd "$ROOT/rust" && cargo test --release -p migrator-core --test go_rust_scenario_integration go_rust_scenario_matches_go_reference -- --nocapture --test-threads=1)
  echo "blocked_table_plan: PASS" >> "$REPORT"
}

echo "== go-rust e2e ALL: full matrix on .temp/sql =="

run_plan_scenario empty_db_plan \
  "$ARTIFACTS/go_e2e_empty_db_plan.json" \
  "$ARTIFACTS/rust_e2e_empty_db_plan.json" \
  0

run_gate_scenario \
  "$ARTIFACTS/go_e2e_prod_gate_cold.json" \
  "$ARTIFACTS/rust_e2e_prod_gate_cold.json"

run_apply_go_export "$ARTIFACTS/go_e2e_apply_smoke.json"

run_plan_scenario warm_db_plan \
  "$ARTIFACTS/go_e2e_warm_db_plan.json" \
  "$ARTIFACTS/rust_e2e_warm_db_plan.json" \
  1

run_plan_scenario skip_unchanged_plan \
  "$ARTIFACTS/go_e2e_skip_unchanged_plan.json" \
  "$ARTIFACTS/rust_e2e_skip_unchanged_plan.json" \
  1

export RMIG_CATALOG_CACHE=1
run_plan_scenario catalog_cache_plan \
  "$ARTIFACTS/go_e2e_catalog_cache_plan.json" \
  "$ARTIFACTS/rust_e2e_catalog_cache_plan.json" \
  1
unset RMIG_CATALOG_CACHE
export RMIG_CATALOG_CACHE=0

run_blocked_scenario \
  "$ARTIFACTS/go_e2e_blocked_table_plan.json" \
  "$ARTIFACTS/rust_e2e_blocked_table_plan.json"

run_apply_rust_compare \
  "$ARTIFACTS/go_e2e_apply_smoke.json" \
  "$ARTIFACTS/rust_e2e_apply_smoke.json"

{
  echo "go-rust e2e ALL: PASS"
  cat "$REPORT"
} | tee "$ARTIFACTS/go_rust_e2e_all_report.txt"

echo "Report: $ARTIFACTS/go_rust_e2e_all_report.txt"
