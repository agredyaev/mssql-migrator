#!/usr/bin/env bash
# Summarize committed perf artifacts (regenerate with make bench-footprint* first).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ART="${RMIG_FOOTPRINT_ARTIFACTS:-$ROOT/ops/perf/artifacts}"
OUT="${ART}/profile_summary.txt"
mkdir -p "$ART"

{
  echo "# rmig profile summary - $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "# Regenerate: make bench-footprint-profile && make bench-footprint-alloc"
  echo ""

  if [[ -f "$ART/plan_diff_5k_flamegraph.svg" ]]; then
    echo "== CPU flamegraph (5k skip-heavy diff) =="
    echo "  $ART/plan_diff_5k_flamegraph.svg"
    echo ""
  fi

  for f in plan_diff_dhat.txt plan_diff_dhat_transitions.txt plan_diff_dhat_scan.txt; do
    if [[ -f "$ART/$f" ]]; then
      echo "== dhat summary: $f =="
      head -n 40 "$ART/$f"
      echo ""
    fi
  done

  for f in alloc_flame.txt alloc_flame_transitions.txt alloc_flame_scan.txt; do
    if [[ -f "$ART/$f" ]]; then
      echo "== alloc tree: $f =="
      head -n 30 "$ART/$f"
      echo ""
    fi
  done

  if [[ -f "$ART/footprint_bench.txt" ]]; then
    echo "== criterion bench log (tail) =="
    tail -n 15 "$ART/footprint_bench.txt"
    echo ""
  fi
} | tee "$OUT"

echo "Wrote $OUT"
