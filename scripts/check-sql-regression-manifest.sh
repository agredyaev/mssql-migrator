#!/usr/bin/env bash
# sql_regression.sh must list every SQL integration crate with bugslog-style coverage.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
REGRESSION="$ROOT/ops/perf/sql_regression.sh"

fail() {
  echo "check-sql-regression-manifest: $*" >&2
  exit 1
}

[[ -f "$REGRESSION" ]] || fail "missing $REGRESSION"

REQUIRED=(
  advisory_lock_guard_test
  advisory_lock_rmigd_test
  multi_db_plan_test
  plan_deferred_bootstrap_test
  plan_collation_test
  plan_master_access_test
  plan_no_db_side_effect_test
  session_fallback_test
  db_auth_test
  apply_e2e_integration
  existing_db_adoption_integration
  adopt_e2e_integration
  drift_e2e_integration
  chaos_kill_mid_apply_test
)

for test in "${REQUIRED[@]}"; do
  grep -q "$test" "$REGRESSION" \
    || fail "happy path: sql_regression.sh must run $test"
done

if ! grep -q 'cargo test --release -p rmig -- --nocapture --test-threads=1' "$REGRESSION"; then
  fail "happy path: sql_regression.sh must run the rmig CLI test suite"
fi

if ! grep -q 'RMIG_USE_RMIGD' "$REGRESSION"; then
  fail "edge case: sql_regression must enable RMIG_USE_RMIGD for rmigd suites"
fi

if ! grep -q 'cargo build --release -p rmigd' "$REGRESSION"; then
  fail "negative path: sql_regression must build rmigd before daemon tests"
fi

if ! grep -q 'RMIG_RUN_SQLSERVER_INTEGRATION' "$ROOT/ops/perf/e2e_env.sh"; then
  fail "BG-001 regression: e2e_env.sh must set RMIG_RUN_SQLSERVER_INTEGRATION"
fi

echo "check-sql-regression-manifest: PASS"
