#!/usr/bin/env bash
# Verify e2e scenario matrix matches committed baselines and Rust harness.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

BASELINE_DIR="$ROOT/crates/core/tests/testdata/e2e"
RUST_TEST="$ROOT/crates/core/tests/scenario_e2e_integration.rs"
E2E_ALL="$ROOT/ops/perf/e2e_all.sh"

declare -a BASELINE_SCENARIOS=()
for f in "$BASELINE_DIR"/e2e_baseline_*.json; do
  [[ -f "$f" ]] || continue
  name="$(basename "$f" .json)"
  name="${name#e2e_baseline_}"
  BASELINE_SCENARIOS+=("$name")
done

if ((${#BASELINE_SCENARIOS[@]} == 0)); then
  echo "check-e2e-scenarios: no baselines under $BASELINE_DIR" >&2
  exit 1
fi

# Scenarios orchestrated by e2e_all.sh (order preserved for doc only).
declare -a E2E_ALL_SCENARIOS=(
  empty_db_plan
  prod_gate_cold
  warm_db_plan
  skip_unchanged_plan
  catalog_cache_plan
  blocked_table_plan
  apply_smoke_result
  ddl_transition_apply
)

fail=0
report() {
  echo "check-e2e-scenarios: $*" >&2
  fail=1
}

for s in "${E2E_ALL_SCENARIOS[@]}"; do
  baseline="$BASELINE_DIR/e2e_baseline_${s}.json"
  if [[ ! -f "$baseline" ]]; then
    report "missing baseline for e2e_all scenario '$s': $baseline"
  fi
  if ! grep -q "\"$s\"" "$RUST_TEST" 2>/dev/null && ! grep -q "$s" "$RUST_TEST"; then
    report "Rust harness missing scenario '$s' in $RUST_TEST"
  fi
done

for s in "${BASELINE_SCENARIOS[@]}"; do
  found=0
  for e in "${E2E_ALL_SCENARIOS[@]}"; do
    [[ "$e" == "$s" ]] && found=1 && break
  done
  if [[ "$found" -eq 0 ]]; then
    report "orphan baseline (not in e2e_all.sh): e2e_baseline_${s}.json"
  fi
done

# Plan scenarios must appear in is_plan_scenario match list.
PLAN_SCENARIOS=(empty_db_plan warm_db_plan skip_unchanged_plan catalog_cache_plan)
for s in "${PLAN_SCENARIOS[@]}"; do
  if ! grep -q "$s" "$RUST_TEST"; then
    report "plan scenario '$s' not referenced in $RUST_TEST"
  fi
done

if [[ "$fail" -ne 0 ]]; then
  exit 1
fi

echo "check-e2e-scenarios: OK (${#BASELINE_SCENARIOS[@]} baselines, ${#E2E_ALL_SCENARIOS[@]} e2e_all scenarios)"
