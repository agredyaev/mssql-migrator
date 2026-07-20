#!/usr/bin/env bash
# Plan DB phase perf: workflow integration + trace JSON (parallel_wall_ms SLO 500ms).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
# shellcheck source=ops/perf/e2e_env.sh
source "$ROOT/ops/perf/e2e_env.sh"
ARTIFACTS="$ROOT/ops/perf/artifacts"
mkdir -p "$ARTIFACTS"
rm -f "$ARTIFACTS/plan_db_trace.json"

export RMIG_RUN_SQLSERVER_INTEGRATION="${RMIG_RUN_SQLSERVER_INTEGRATION:-1}"
# Force tracing on: this target's whole point is the trace artifact, so a
# caller's RMIG_PLAN_DB_TRACE=0 must not silently disable the evidence.
export RMIG_PLAN_DB_TRACE=1
export RMIG_PLAN_DB_MAX_PAR_MS="${RMIG_PLAN_DB_MAX_PAR_MS:-500}"
export RMIG_REPO_ROOT="$ROOT"

cargo test --profile release-fast -p migrator-core --test workflow_integration \
  -- --nocapture --test-threads=1

if [ ! -s "$ARTIFACTS/plan_db_trace.json" ]; then
  echo "ERROR: this run wrote no plan_db_trace.json — trace evidence missing" >&2
  exit 1
fi
echo "trace: $ARTIFACTS/plan_db_trace.json"
