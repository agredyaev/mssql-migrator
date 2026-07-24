#!/usr/bin/env bash
# Shared integration/e2e env defaults (must match docker-compose.yml MSSQL_SA_PASSWORD).
# Source from ops/perf/*.sh after setting ROOT to repo root.
#
# Usage:
#   ROOT="$(cd ... && pwd)"
#   # shellcheck source=ops/perf/e2e_env.sh
#   source "$ROOT/ops/perf/e2e_env.sh"

: "${ROOT:?set ROOT to repo root before sourcing e2e_env.sh}"

export RMIG_RUN_SQLSERVER_INTEGRATION="${RMIG_RUN_SQLSERVER_INTEGRATION:-1}"
# Runner-invoked suites must fail loud instead of silently skipping if the
# integration toggle above is lost somewhere in the plumbing.
export RMIG_REQUIRE_INTEGRATION="${RMIG_REQUIRE_INTEGRATION:-1}"
echo "e2e env: loadavg $(sysctl -n vm.loadavg 2>/dev/null || cat /proc/loadavg 2>/dev/null || echo unknown)" >&2
export RM_DB_SERVER="${RM_DB_SERVER:-localhost}"
case "$RM_DB_SERVER" in
  localhost|127.0.0.1|::1) ;;
  *)
    printf 'e2e env: refusing non-loopback RM_DB_SERVER=%q\n' "$RM_DB_SERVER" >&2
    exit 1
    ;;
esac
export RM_DB_PORT="${RM_DB_PORT:-1433}"
export RM_DB_USER="${RM_DB_USER:-sa}"
export RM_DB_PASSWORD="${RM_DB_PASSWORD:-yourStrong(!)Password}"
export RM_DB_ENCRYPT="${RM_DB_ENCRYPT:-false}"
export RM_DB_TRUST_SERVER_CERTIFICATE="${RM_DB_TRUST_SERVER_CERTIFICATE:-true}"
export RM_SQL_ROOT="${RM_SQL_ROOT:-$ROOT/.temp/sql}"
export RM_SQL_BASE="${RM_SQL_BASE:-$ROOT/.temp/sql}"
export RMIG_E2E_TIMING_FACTOR="${RMIG_E2E_TIMING_FACTOR:-3.0}"
export RMIG_E2E_TIMING_SLACK_MS="${RMIG_E2E_TIMING_SLACK_MS:-100}"
export RMIG_CATALOG_CACHE="${RMIG_CATALOG_CACHE:-0}"

# Discover sole catalog database under RM_SQL_ROOT (mirrors Rust discover_catalog_databases).
discover_catalog_db() {
  local root="$1"
  local -a dbs=()
  local name sub
  if [[ ! -d "$root" ]]; then
    return 1
  fi
  for sub in "$root"/*; do
    [[ -d "$sub" ]] || continue
    name="$(basename "$sub")"
    [[ "$name" == .* ]] && continue
    for _ in "$sub"/*; do
      [[ -d "$_" ]] && dbs+=("$name") && break
    done
  done
  if ((${#dbs[@]} == 1)); then
    echo "${dbs[0]}"
    return 0
  fi
  return 1
}

if [[ -z "${RM_DB_DATABASE:-}" ]]; then
  if db="$(discover_catalog_db "$RM_SQL_ROOT")"; then
    export RM_DB_DATABASE="$db"
  else
    export RM_DB_DATABASE="${RM_DB_DATABASE:-dactests}"
  fi
fi
