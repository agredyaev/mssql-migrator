#!/usr/bin/env bash
# Plan scenario subset on .temp/sql (empty_db_plan + warm_db_plan).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
# shellcheck source=ops/perf/e2e_env.sh
source "$ROOT/ops/perf/e2e_env.sh"
ARTIFACTS="$ROOT/ops/perf/artifacts"
BASELINE="$ROOT/crates/core/tests/testdata/e2e"
mkdir -p "$ARTIFACTS"

export RM_SKIP_GIT="${RM_SKIP_GIT:-1}"

run_scenario() {
  local scenario="$1"
  local skip_reset="${2:-1}"
  export RMIG_E2E_SCENARIO="$scenario"
  export RMIG_E2E_BASELINE_REPORT="$BASELINE/e2e_baseline_${scenario}.json"
  export RMIG_E2E_REPORT="$ARTIFACTS/e2e_${scenario}.json"
  if [[ "$skip_reset" == "0" ]]; then
    unset RMIG_GATE_SKIP_DB_RESET
  else
    export RMIG_GATE_SKIP_DB_RESET=1
  fi
  echo "== scenario ${scenario} =="
  cargo test --release -p migrator-core --test scenario_e2e_integration scenario_matches_baseline -- --nocapture --test-threads=1
}

echo "== e2e: plan scenarios on .temp/sql =="

run_scenario empty_db_plan 0

echo "== apply smoke baseline =="
export RMIG_GATE_SKIP_DB_RESET=1
cargo test --release -p migrator-core --test scenario_e2e_integration apply_smoke_setup -- --nocapture --test-threads=1

run_scenario warm_db_plan 1

echo "e2e scenarios: PASS"
echo "Report: $ARTIFACTS/e2e_report.txt"
{
  echo "e2e scenarios: PASS"
  echo "timing factor=${RMIG_E2E_TIMING_FACTOR} slack_ms=${RMIG_E2E_TIMING_SLACK_MS}"
} | tee "$ARTIFACTS/e2e_report.txt"
