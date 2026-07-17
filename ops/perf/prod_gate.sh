#!/usr/bin/env bash
# Prod incremental go/no-go gate (plan snapshot vs committed baseline).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
ARTIFACTS="$ROOT/ops/perf/artifacts"
mkdir -p "$ARTIFACTS"

export RMIG_RUN_SQLSERVER_INTEGRATION="${RMIG_RUN_SQLSERVER_INTEGRATION:-1}"
export RM_SKIP_GIT="${RM_SKIP_GIT:-1}"
export RM_DB_SERVER="${RM_DB_SERVER:-localhost}"
export RM_DB_PORT="${RM_DB_PORT:-1433}"
export RM_DB_USER="${RM_DB_USER:-sa}"
export RM_DB_PASSWORD="${RM_DB_PASSWORD:-yourStrong(!)Password}"
export RM_DB_ENCRYPT="${RM_DB_ENCRYPT:-false}"
export RM_DB_TRUST_SERVER_CERTIFICATE="${RM_DB_TRUST_SERVER_CERTIFICATE:-true}"
export RM_SQL_ROOT="${RM_SQL_ROOT:-$ROOT/.temp/sql}"
export RMIG_GATE_REPORT="$ARTIFACTS/prod_gate_report.json"

if [ "${RMIG_GATE_SKIP_DB_RESET:-}" != "1" ]; then
  echo "reset catalog databases under ${RM_SQL_ROOT}..."
  for db in "$RM_SQL_ROOT"/*/; do
    [ -d "$db" ] || continue
    name="$(basename "$db")"
    [[ "$name" == .* ]] && continue
    # Directory names reach privileged T-SQL: refuse anything that could
    # escape the literal/identifier context.
    if ! [[ "$name" =~ ^[A-Za-z0-9_]+$ ]]; then
      echo "ERROR: unsafe catalog directory name (allowed: [A-Za-z0-9_]+): $name" >&2
      exit 1
    fi
    docker compose -f "$ROOT/docker-compose.yml" exec -T mssql /opt/mssql-tools18/bin/sqlcmd \
      -S "$RM_DB_SERVER" -U "$RM_DB_USER" -P "$RM_DB_PASSWORD" -C \
      -Q "IF DB_ID('${name}') IS NOT NULL BEGIN ALTER DATABASE [${name}] SET SINGLE_USER WITH ROLLBACK IMMEDIATE; DROP DATABASE [${name}]; END; CREATE DATABASE [${name}];"
  done
fi

cargo build --release -p rmigd
cargo test --release -p migrator-core --test prod_gate_integration prod_gate_incremental_plan -- --nocapture --test-threads=1

echo "Gate report: ${RMIG_GATE_REPORT}"
