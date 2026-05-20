#!/usr/bin/env bash
# Go↔Rust DB I/O debug cycle: phase timings + driver boundary stats on .temp/sql.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
ARTIFACTS="$ROOT/ops/perf/artifacts"
mkdir -p "$ARTIFACTS"

export RMIG_RUN_SQLSERVER_INTEGRATION=1
export RM_SKIP_GIT=1
export RMIG_CATALOG_CACHE=0
export RMIG_E2E_RUN_ID="${RMIG_E2E_RUN_ID:-io-$(date +%s)}"
export RM_DB_SERVER="${RM_DB_SERVER:-localhost}"
export RM_DB_PORT="${RM_DB_PORT:-1433}"
export RM_DB_DATABASE="${RM_DB_DATABASE:-rmig_test}"
export RM_DB_USER="${RM_DB_USER:-sa}"
export RM_DB_PASSWORD="${RM_DB_PASSWORD:-yourStrong(!)Password}"
export RM_DB_ENCRYPT="${RM_DB_ENCRYPT:-false}"
export RM_DB_TRUST_SERVER_CERTIFICATE="${RM_DB_TRUST_SERVER_CERTIFICATE:-true}"
export RM_SQL_ROOT="${RM_SQL_ROOT:-$ROOT/.temp/sql}"

SUMMARY="$ARTIFACTS/go_rust_io_debug_summary.txt"
: > "$SUMMARY"

run_scenario() {
  local scenario="$1"
  local go_report="$2"
  local rust_report="$3"

  echo "== io debug: ${scenario} Go =="
  export RMIG_E2E_SCENARIO="$scenario"
  export RMIG_E2E_EXPORT_REPORT="$go_report"
  if [[ "$scenario" == "empty_db_plan" ]]; then
    unset RMIG_GATE_SKIP_DB_RESET
  else
    export RMIG_GATE_SKIP_DB_RESET=1
  fi
  go test -tags=integration ./internal/app/ -run TestE2E_ExportScenarioReport -v -count=1 2>&1 | tail -5

  echo "== io debug: ${scenario} Rust =="
  export RMIG_E2E_GO_REPORT="$go_report"
  export RMIG_E2E_RUST_REPORT="$rust_report"
  export RMIG_GATE_SKIP_DB_RESET=1
  (cd "$ROOT/rust" && cargo test --release -p migrator-core --test go_rust_scenario_integration go_rust_scenario_matches_go_reference -- --nocapture --test-threads=1)

  python3 - "$go_report" "$rust_report" >> "$SUMMARY" <<'PY'
import json, sys
go_path, rust_path = sys.argv[1:3]
go = json.load(open(go_path))
rust = json.load(open(rust_path))
gt, rt = go["timings"], rust["timings"]
gio, rio = go.get("io", {}), rust.get("io", {})

def row(name, gv, rv):
    delta = rv - gv
    flag = " SLOWER" if delta > 50 else (" faster" if delta < -50 else "")
    return f"{name:22} go={gv:5}ms rust={rv:5}ms delta={delta:+5}ms{flag}"

lines = [
    f"io debug summary ({go['scenario']})",
    "",
    "Phase wall (timings):",
    row("connect_ms", gt.get("connect_ms", 0), rt.get("connect_ms", 0)),
    row("scan_ms", gt.get("scan_ms", 0), rt.get("scan_ms", 0)),
    row("ensure_ms", gt.get("ensure_ms", 0), rt.get("ensure_ms", 0)),
    row("checksums_ms", gt.get("checksums_ms", 0), rt.get("checksums_ms", 0)),
    row("inspect_ms", gt.get("inspect_ms", 0), rt.get("inspect_ms", 0)),
    row("parallel_wall_ms", gt.get("parallel_wall_ms", 0), rt.get("parallel_wall_ms", 0)),
    row("diff_ms", gt.get("diff_ms", 0), rt.get("diff_ms", 0)),
    row("plan_wall_ms", gt.get("plan_wall_ms", 0), rt.get("plan_wall_ms", 0)),
    "",
    "Driver boundary (io):",
    row("query_ms", gio.get("query_ms", 0), rio.get("query_ms", 0)),
    f"{'query_calls':22} go={gio.get('query_calls', 0):5} rust={rio.get('query_calls', 0):5}",
    row("exec_ms", gio.get("exec_ms", 0), rio.get("exec_ms", 0)),
    f"{'exec_calls':22} go={gio.get('exec_calls', 0):5} rust={rio.get('exec_calls', 0):5}",
    f"{'extra_connects':22} go={gio.get('extra_connects', 0):5} rust={rio.get('extra_connects', 0):5}",
    "",
    f"Go db_boundary_ms={gio.get('query_ms',0)+gio.get('fetch_ms',0)+gio.get('exec_ms',0)}",
    f"Rust db_boundary_ms={rio.get('query_ms',0)+rio.get('exec_ms',0)}",
    "",
]
print("\n".join(lines))
PY
}

GO_EMPTY="$ARTIFACTS/go_io_debug_empty.json"
RUST_EMPTY="$ARTIFACTS/rust_io_debug_empty.json"
run_scenario empty_db_plan "$GO_EMPTY" "$RUST_EMPTY"

echo "== apply smoke baseline (Go) for warm_db_plan =="
export RMIG_GATE_SKIP_DB_RESET=1
go test -tags=integration ./internal/app/ -run TestE2E_ApplySmokeBaseline -v -count=1 2>&1 | tail -3

GO_WARM="$ARTIFACTS/go_io_debug_warm.json"
RUST_WARM="$ARTIFACTS/rust_io_debug_warm.json"
run_scenario warm_db_plan "$GO_WARM" "$RUST_WARM"

cat "$SUMMARY"
echo "Summary: $SUMMARY"
