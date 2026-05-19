#!/usr/bin/env bash
# Print CPU and heap top lines from pprof artifacts (regenerate profiles first if missing).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ART="${RMIG_FOOTPRINT_ARTIFACTS:-$ROOT/ops/perf/artifacts}"
OUT="${ART}/profile_summary.txt"
mkdir -p "$ART"

{
  echo "# rmig profile summary — $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "# Regenerate: make bench-footprint-profile && ops/perf/cli_phase.sh profile"
  echo ""

  report() {
    local label="$1" cpu="$2" mem="$3"
    echo "== $label =="
    if [[ -f "$cpu" ]]; then
      echo "--- CPU ($cpu) ---"
      go tool pprof -top -nodecount=20 "$cpu" 2>/dev/null || echo "(failed to read cpu profile)"
    else
      echo "--- CPU: missing $cpu ---"
    fi
    echo ""
    if [[ -f "$mem" ]]; then
      echo "--- heap alloc_space ($mem) ---"
      go tool pprof -top -nodecount=20 -alloc_space "$mem" 2>/dev/null || echo "(failed to read mem profile)"
    else
      echo "--- MEM: missing $mem ---"
    fi
    echo ""
  }

  report "in-process diff 5k (footprint)" \
    "$ART/footprint_5k.cpu.prof" "$ART/footprint_5k.mem.prof"

  report "full CLI plan cold (SQL integration)" \
    "$ART/cli_plan.cpu.prof" "$ART/cli_plan.mem.prof"
} | tee "$OUT"

echo "Wrote $OUT"
