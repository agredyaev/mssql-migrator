#!/usr/bin/env bash
# Prod incremental go/no-go gate against SQL Server (Docker via make db-up).
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

REPORT="${RMIG_GATE_REPORT:-$ROOT/ops/perf/artifacts/prod_gate_report.json}"
mkdir -p "$(dirname "$REPORT")"

export RMIG_RUN_SQLSERVER_INTEGRATION RM_DB_SERVER RM_DB_PORT RM_DB_DATABASE
export RM_DB_USER RM_DB_PASSWORD RM_DB_ENCRYPT RM_DB_TRUST_SERVER_CERTIFICATE
export RMIG_GATE_REPORT="$REPORT"

# Closer to prod: skip DROP/CREATE when RMIG_GATE_SKIP_DB_RESET=1
go test -tags=integration ./internal/app/ -run TestProdGate_IncrementalPlan -v -count=1 "$@"

echo "Gate report: $REPORT"
