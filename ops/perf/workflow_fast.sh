#!/usr/bin/env bash
# Fast full workflow: truncate smoke + audit instead of DROP/CREATE (~3s → ~300ms reset).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

export RMIG_RUN_SQLSERVER_INTEGRATION="${RMIG_RUN_SQLSERVER_INTEGRATION:-1}"
export RMIG_WORKFLOW_FAST_RESET="${RMIG_WORKFLOW_FAST_RESET:-1}"
export RMIG_PLAN_DB_MAX_PAR_MS="${RMIG_PLAN_DB_MAX_PAR_MS:-500}"
export RMIG_REPO_ROOT="$ROOT"
export RM_SQL_ROOT="${RM_SQL_ROOT:-$ROOT/.temp/sql}"
export RM_SQL_BASE="${RM_SQL_BASE:-$ROOT/.temp/sql}"

cargo test --profile release-fast -p migrator-core --test workflow_integration \
  -- --nocapture --test-threads=1
