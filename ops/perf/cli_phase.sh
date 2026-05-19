#!/usr/bin/env bash
# Full CLI phase timings (plan + migrate) against Docker SQL Server.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

: "${RMIG_RUN_SQLSERVER_INTEGRATION:=1}"
: "${RM_DB_SERVER:=localhost}"
: "${RM_DB_PORT:=1433}"
: "${RM_DB_DATABASE:=rmig_test}"
: "${RM_DB_USER:=sa}"
: "${RM_DB_PASSWORD:=yourStrong(!)Password}"
: "${RM_DB_ENCRYPT:=false}"
: "${RM_DB_TRUST_SERVER_CERTIFICATE:=true}"

ARTIFACTS="$ROOT/ops/perf/artifacts"
mkdir -p "$ARTIFACTS"

export RMIG_RUN_SQLSERVER_INTEGRATION RM_DB_SERVER RM_DB_PORT RM_DB_DATABASE
export RM_DB_USER RM_DB_PASSWORD RM_DB_ENCRYPT RM_DB_TRUST_SERVER_CERTIFICATE

MODE="${1:-cold}"
shift || true

case "$MODE" in
  cold)
    export RMIG_CLI_PHASE_REPORT="$ARTIFACTS/cli_phase_plan_cold.json"
    go test -tags=integration ./internal/app/ \
      -run TestIntegration_PhaseReport_CLI_Plan -v -count=1 "$@"
  ;;
  warm)
    export RMIG_PHASE_SKIP_DB_RESET=1
    export RMIG_CLI_PHASE_REPORT="$ARTIFACTS/cli_phase_plan_warm.json"
    go test -tags=integration ./internal/app/ \
      -run TestIntegration_PhaseReport_CLI_Plan -v -count=1 "$@"
  ;;
  migrate-cold)
    export RMIG_CLI_PHASE_REPORT="$ARTIFACTS/cli_phase_migrate_cold.json"
    go test -tags=integration ./internal/app/ \
      -run TestIntegration_PhaseReport_CLI_Migrate -v -count=1 "$@"
  ;;
  profile)
    export RMIG_CLI_PHASE_REPORT="$ARTIFACTS/cli_phase_plan_cold.json"
    go test -tags=integration ./internal/app/ \
      -run TestIntegration_PhaseReport_CLI_Plan -v -count=1 \
      -cpuprofile="$ARTIFACTS/cli_plan.cpu.prof" \
      -memprofile="$ARTIFACTS/cli_plan.mem.prof" \
      -memprofilerate=1 \
      -trace="$ARTIFACTS/cli_plan.trace" \
      "$@"
    echo "Profiles: $ARTIFACTS/cli_plan.cpu.prof (mostly idle/wait on SQL Server)"
    echo "         $ARTIFACTS/cli_plan.mem.prof"
    echo "Inspect fetch: see fetch_ms in $RMIG_CLI_PHASE_REPORT"
    ;;
  *)
    echo "usage: $0 {cold|warm|migrate-cold|profile} [go test args...]" >&2
    exit 2
    ;;
esac

echo "Phase report: ${RMIG_CLI_PHASE_REPORT:-<not set>}"
