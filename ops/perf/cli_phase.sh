#!/usr/bin/env bash
# Rust plan phase timings + SLO gate (cli_wall_ms < 150 on cache-miss, rmigd session).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
# shellcheck source=ops/perf/e2e_env.sh
source "$ROOT/ops/perf/e2e_env.sh"
ARTIFACTS="$ROOT/ops/perf/artifacts"
mkdir -p "$ARTIFACTS"

export RMIG_RUN_SQLSERVER_INTEGRATION="${RMIG_RUN_SQLSERVER_INTEGRATION:-1}"
export RM_SKIP_GIT="${RM_SKIP_GIT:-1}"
export RMIG_SLO_MAX_CLI_WALL_MS="${RMIG_SLO_MAX_CLI_WALL_MS:-150}"
export RMIG_USE_RMIGD="${RMIG_USE_RMIGD:-1}"
export RMIG_SESSION_TOKEN="${RMIG_SESSION_TOKEN:-rmig-integration-test-token}"
export RMIG_INTEGRATION_WARM_SNAPSHOT="${RMIG_INTEGRATION_WARM_SNAPSHOT:-1}"

MODE="${1:-slo}"
case "$MODE" in
  slo|warm|all)
    export RMIG_CLI_PHASE_REPORT="$ARTIFACTS/cli_phase_slo.json"
    cargo build --release -p rmigd
    cargo test --release -p migrator-core --test integration_plan integration_plan_sqlserver_suite -- --nocapture --test-threads=1
    ;;
  *)
    echo "usage: $0 {slo|warm|all}" >&2
    exit 1
    ;;
esac
