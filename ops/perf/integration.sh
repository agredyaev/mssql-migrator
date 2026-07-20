#!/usr/bin/env bash
# Rust-only SQL Server e2e: object creation + audit metadata + git workflow edits.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
# shellcheck source=ops/perf/e2e_env.sh
source "$ROOT/ops/perf/e2e_env.sh"

export RMIG_WORKFLOW_FAST_RESET="${RMIG_WORKFLOW_FAST_RESET:-1}"
export RMIG_PLAN_DB_MAX_PAR_MS="${RMIG_PLAN_DB_MAX_PAR_MS:-500}"
export RMIG_REPO_ROOT="$ROOT"

echo "== e2e: cold apply + audit =="
cargo test --profile release-fast -p migrator-core --test apply_e2e_integration \
  -- --nocapture --test-threads=1

echo "== e2e: adopt pre-existing identical object =="
cargo test --profile release-fast -p migrator-core --test adopt_e2e_integration \
  -- --nocapture --test-threads=1

echo "== e2e: drift lifecycle (oob drop/modify, fail-retry, history) =="
cargo test --profile release-fast -p migrator-core --test drift_e2e_integration \
  -- --nocapture --test-threads=1

echo "== e2e: git workflow (create / DDL / view edit) =="
cargo test --profile release-fast -p migrator-core --test workflow_integration \
  -- --nocapture --test-threads=1

echo "integration: PASS"
