#!/usr/bin/env bash
# Full e2e matrix on .temp/sql + Docker SQL Server vs committed baselines.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
# shellcheck source=ops/perf/e2e_env.sh
source "$ROOT/ops/perf/e2e_env.sh"
ARTIFACTS="$ROOT/ops/perf/artifacts"
BASELINE="$ROOT/crates/core/tests/testdata/e2e"
mkdir -p "$ARTIFACTS"

export RM_SKIP_GIT="${RM_SKIP_GIT:-1}"

REPORT="$ARTIFACTS/e2e_all_report.txt"
: > "$REPORT"

run_scenario() {
  local scenario="$1"
  local skip_reset="${2:-1}"

  echo "== scenario ${scenario} =="
  export RMIG_E2E_SCENARIO="$scenario"
  export RMIG_E2E_BASELINE_REPORT="$BASELINE/e2e_baseline_${scenario}.json"
  export RMIG_E2E_REPORT="$ARTIFACTS/e2e_${scenario}.json"
  if [[ "$skip_reset" == "0" ]]; then
    unset RMIG_GATE_SKIP_DB_RESET
  else
    export RMIG_GATE_SKIP_DB_RESET=1
  fi
  cargo test --release -p migrator-core --test scenario_e2e_integration scenario_matches_baseline -- --nocapture --test-threads=1
  echo "${scenario}: PASS" >> "$REPORT"
}

run_apply_baseline() {
  echo "== apply smoke baseline (warm setup) =="
  export RMIG_GATE_SKIP_DB_RESET=1
  cargo test --release -p migrator-core --test scenario_e2e_integration apply_smoke_setup -- --nocapture --test-threads=1
}

echo "== e2e ALL: full matrix on .temp/sql =="

run_scenario empty_db_plan 0

export RMIG_GATE_SKIP_DB_RESET=1
run_scenario prod_gate_cold 1

run_apply_baseline

run_scenario warm_db_plan 1
run_scenario skip_unchanged_plan 1

export RMIG_CATALOG_CACHE=1
run_scenario catalog_cache_plan 1
unset RMIG_CATALOG_CACHE
export RMIG_CATALOG_CACHE=0

# Reuse warm DB from catalog_cache_plan (no DROP); setup_apply_ms must stay under 500ms.
export RMIG_GATE_SKIP_DB_RESET=1
run_scenario blocked_table_plan 1

# Cold apply resets DB; run before ddl_transition so the matrix ends with migration history in SQL.
unset RMIG_GATE_SKIP_DB_RESET
export RMIG_CATALOG_CACHE=1
run_scenario apply_smoke_result 0

unset RM_SKIP_GIT
export RMIG_CATALOG_CACHE=1
export RMIG_GATE_SKIP_DB_RESET=1
run_scenario ddl_transition_apply 1
export RM_SKIP_GIT="${RM_SKIP_GIT:-1}"
unset RMIG_CATALOG_CACHE
export RMIG_CATALOG_CACHE=0

# Never read and truncate the same file in one pipeline: build the final
# artifact in a temp file, then atomically replace the report.
FINAL="$(mktemp "$ARTIFACTS/e2e_all_report.XXXXXX")"
{
  echo "e2e ALL: PASS"
  cat "$REPORT"
} > "$FINAL"
mv "$FINAL" "$ARTIFACTS/e2e_all_report.txt"
cat "$ARTIFACTS/e2e_all_report.txt"

echo "Report: $ARTIFACTS/e2e_all_report.txt"
