#!/usr/bin/env bash
# Bugslog SQL regression battery (requires Docker MSSQL).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
# shellcheck source=ops/perf/e2e_env.sh
source "$ROOT/ops/perf/e2e_env.sh"

export RMIG_USE_RMIGD="${RMIG_USE_RMIGD:-1}"
export RMIG_SESSION_TOKEN="${RMIG_SESSION_TOKEN:-rmig-integration-test-token}"
mkdir -p "$ROOT/.rmig"
export RMIGD_SOCKET="${RMIGD_SOCKET:-$ROOT/.rmig/rmigd-sql-regression.sock}"
LOCK_DIR="$ROOT/.rmig/sql-regression.lock"
LOCK_PID="$LOCK_DIR/pid"

debug_log() {
  true
}

debug_log_failure() {
  true
}

on_error() {
  local status="$?"
  debug_log_failure "$status" "${BASH_COMMAND:-}"
  exit "$status"
}

cleanup_lock() {
  if [ -d "$LOCK_DIR" ] && [ "$(cat "$LOCK_PID" 2>/dev/null || true)" = "$$" ]; then
    rm -rf "$LOCK_DIR"
  fi
}

claim_lock() {
  if mkdir "$LOCK_DIR" 2>/dev/null; then
    printf '%s\n' "$$" > "$LOCK_PID"
    return 0
  fi

  holder_pid="$(cat "$LOCK_PID" 2>/dev/null || true)"
  if [ -n "$holder_pid" ] && kill -0 "$holder_pid" 2>/dev/null; then
    # #region agent log
    debug_log "H12" "ops/perf/sql_regression.sh:claim_lock" "sql_regression lock already held" "lock-busy"
    # #endregion
    echo "sql-regression: another run is active (pid $holder_pid, lock $LOCK_DIR)" >&2
    exit 1
  fi

  rm -rf "$LOCK_DIR"
  mkdir "$LOCK_DIR"
  printf '%s\n' "$$" > "$LOCK_PID"
}

trap cleanup_lock EXIT INT TERM HUP
trap on_error ERR
# #region agent log
debug_log "H10" "ops/perf/sql_regression.sh:entry" "sql_regression script invoked" "entry"
printf 'sql_regression debug session=1200a9 run_id=%s\n' "${RMIG_DEBUG_RUN_ID:-manual}" >&2
# #endregion
claim_lock
# #region agent log
debug_log "H12" "ops/perf/sql_regression.sh:claim_lock" "sql_regression lock claimed" "lock-claimed"
# #endregion

# Orphaned rmigd processes keep warm TDS sessions and can hold advisory locks.
if command -v lsof >/dev/null 2>&1; then
  while read -r pid; do
    [[ -n "$pid" ]] && kill "$pid" 2>/dev/null || true
  done < <(lsof -t "$RMIGD_SOCKET" 2>/dev/null || true)
fi
pkill -f "${ROOT}/target/release/rmigd" 2>/dev/null || true
rm -f "$RMIGD_SOCKET"
sleep 0.2

run_suite() {
  local label="$1"
  shift
  echo "== sql-regression: ${label} =="
  "$@"
  echo "${label}: PASS"
}

# #region agent log
debug_log "H11" "ops/perf/sql_regression.sh:build" "sql_regression building rmigd" "build-rmigd"
# #endregion
echo "== sql-regression: build rmigd =="
cargo build --release -p rmigd

CORE_TESTS=(
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
)

CORE_ARGS=()
for t in "${CORE_TESTS[@]}"; do
  CORE_ARGS+=(--test "$t")
done

run_suite "migrator-core" \
  cargo test --release -p migrator-core \
    "${CORE_ARGS[@]}" \
    -- --nocapture --test-threads=1

# #region agent log
debug_log "H13" "ops/perf/sql_regression.sh:rmig-cli" "sql_regression starting rmig-cli suite" "rmig-cli-suite"
# #endregion
run_suite "rmig-cli" \
  cargo test --release -p rmig -- --nocapture --test-threads=1

# #region agent log
debug_log "H13" "ops/perf/sql_regression.sh:done" "sql_regression completed successfully" "done"
# #endregion
echo "sql-regression: ALL PASS"
