#!/usr/bin/env bash
# Bugslog SQL regression battery (requires Docker MSSQL).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
# shellcheck source=ops/perf/e2e_env.sh
source "$ROOT/ops/perf/e2e_env.sh"

export RMIG_USE_RMIGD="${RMIG_USE_RMIGD:-1}"
export RMIG_SESSION_TOKEN="${RMIG_SESSION_TOKEN:-rmig-integration-test-token}"
# rmigd refuses a group/world-accessible socket parent; CI umask makes 755.
mkdir -p "$ROOT/.rmig"
chmod 700 "$ROOT/.rmig"

# Ensure catalog databases exist before rmigd discovers them from RM_SQL_ROOT.
ensure_catalog_databases() {
  local root="$1"
  local user="$2"
  local password="$3"
  local server="$4"
  local port="$5"
  local sub name
  if [[ ! -d "$root" ]]; then return; fi
  # Bootstrap runs sqlcmd INSIDE the local compose container: it can only ever
  # target the local stack, so refuse a remote RM_DB_SERVER instead of
  # silently bootstrapping the wrong SQL Server while tests hit the remote.
  case "$server" in
    localhost|127.0.0.1|::1) ;;
    *)
      echo "sql-regression: RM_DB_SERVER=$server is not the local compose stack;" \
           "create its catalog databases yourself or run against localhost" >&2
      exit 1
      ;;
  esac
  for sub in "$root"/*; do
    [[ -d "$sub" ]] || continue
    name="$(basename "$sub")"
    [[ "$name" == .* ]] && continue
    # Names are interpolated into privileged T-SQL below.
    if ! [[ "$name" =~ ^[A-Za-z0-9_]+$ ]]; then
      echo "sql-regression: unsafe catalog directory name (allowed: [A-Za-z0-9_]+): $name" >&2
      exit 1
    fi
    for _ in "$sub"/*; do
      if [[ -d "$_" ]]; then
        docker compose exec -T mssql /opt/mssql-tools18/bin/sqlcmd \
          -S "localhost,$port" -U "$user" -P "$password" -C \
          -Q "IF DB_ID(N'$name') IS NULL CREATE DATABASE [$name]"
        break
      fi
    done
  done
}
ensure_catalog_databases \
  "$RM_SQL_ROOT" "$RM_DB_USER" "$RM_DB_PASSWORD" "$RM_DB_SERVER" "$RM_DB_PORT"
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
  if [ -d "$LOCK_DIR" ] && [ "$(sed -n '1p' "$LOCK_PID" 2>/dev/null || true)" = "$$" ]; then
    rm -rf "$LOCK_DIR"
  fi
}

proc_start() {
  ps -o lstart= -p "$1" 2>/dev/null | sed 's/^ *//;s/ *$//'
}

write_lock_owner() {
  printf '%s\n%s\n' "$$" "$(proc_start "$$")" > "$LOCK_PID"
}

claim_lock() {
  if mkdir "$LOCK_DIR" 2>/dev/null; then
    write_lock_owner
    return 0
  fi

  holder_pid="$(sed -n '1p' "$LOCK_PID" 2>/dev/null || true)"
  holder_start="$(sed -n '2p' "$LOCK_PID" 2>/dev/null || true)"
  # A live pid alone is not ownership: a reused pid from an unrelated process
  # must not impersonate a regression run forever. Require the recorded start
  # time to match the live process too.
  if [ -n "$holder_pid" ] && kill -0 "$holder_pid" 2>/dev/null \
      && [ -n "$holder_start" ] && [ "$(proc_start "$holder_pid")" = "$holder_start" ]; then
    # #region agent log
    debug_log "H12" "ops/perf/sql_regression.sh:claim_lock" "sql_regression lock already held" "lock-busy"
    # #endregion
    echo "sql-regression: another run is active (pid $holder_pid, lock $LOCK_DIR)" >&2
    exit 1
  fi

  rm -rf "$LOCK_DIR"
  mkdir "$LOCK_DIR"
  write_lock_owner
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
# Cleanup is restricted to sockets under the repo's .rmig/ and to processes
# whose command is rmigd: an env typo must never kill an unrelated service or
# delete an arbitrary file.
case "$RMIGD_SOCKET" in
  "$ROOT/.rmig/"*) ;;
  *)
    echo "sql-regression: RMIGD_SOCKET must live under $ROOT/.rmig/ (got: $RMIGD_SOCKET)" >&2
    exit 1
    ;;
esac
if command -v lsof >/dev/null 2>&1; then
  while read -r pid; do
    [[ -n "$pid" ]] || continue
    if ps -o comm= -p "$pid" 2>/dev/null | grep -q "rmigd"; then
      kill "$pid" 2>/dev/null || true
    else
      echo "sql-regression: refusing to kill non-rmigd pid $pid holding $RMIGD_SOCKET" >&2
      exit 1
    fi
  done < <(lsof -t "$RMIGD_SOCKET" 2>/dev/null || true)
elif [[ -e "$RMIGD_SOCKET" || -S "$RMIGD_SOCKET" ]]; then
  echo "sql-regression: lsof is required to prove ownership of existing $RMIGD_SOCKET" >&2
  exit 1
fi
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
  rmigd_timeout_recovery_test
  multi_db_plan_test
  plan_deferred_bootstrap_test
  plan_collation_test
  plan_master_access_test
  plan_no_db_side_effect_test
  session_fallback_test
  db_auth_test
  apply_integrity_integration
  apply_e2e_integration
  existing_db_adoption_integration
  adopt_e2e_integration
  drift_e2e_integration
  chaos_kill_mid_apply_test
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
