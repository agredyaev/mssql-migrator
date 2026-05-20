#!/usr/bin/env bash
# Rust-only SQL Server e2e: object creation + audit metadata + git workflow edits.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT/rust"

export RMIG_RUN_SQLSERVER_INTEGRATION="${RMIG_RUN_SQLSERVER_INTEGRATION:-1}"
export RMIG_WORKFLOW_FAST_RESET="${RMIG_WORKFLOW_FAST_RESET:-1}"
export RMIG_PLAN_DB_MAX_PAR_MS="${RMIG_PLAN_DB_MAX_PAR_MS:-500}"
export RMIG_REPO_ROOT="$ROOT"
export RM_DB_SERVER="${RM_DB_SERVER:-localhost}"
export RM_DB_PORT="${RM_DB_PORT:-1433}"
export RM_DB_USER="${RM_DB_USER:-sa}"
export RM_DB_PASSWORD="${RM_DB_PASSWORD:-yourStrong(!)Password}"
export RM_DB_ENCRYPT="${RM_DB_ENCRYPT:-false}"
export RM_DB_TRUST_SERVER_CERTIFICATE="${RM_DB_TRUST_SERVER_CERTIFICATE:-true}"
export RM_SQL_ROOT="${RM_SQL_ROOT:-$ROOT/.temp/sql}"
export RM_SQL_BASE="${RM_SQL_BASE:-$ROOT/.temp/sql}"

echo "== rust e2e: cold apply + audit =="
cargo test --profile release-fast -p migrator-core --test apply_e2e_integration \
  -- --nocapture --test-threads=1

echo "== rust e2e: git workflow (create / DDL / view edit) =="
cargo test --profile release-fast -p migrator-core --test workflow_integration \
  -- --nocapture --test-threads=1

echo "rust-e2e: PASS"
