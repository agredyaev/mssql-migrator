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

  for svg in plan_diff_5k_flamegraph.svg rust_plan_diff_5k_flamegraph.svg; do
    if [[ -f "$ART/$svg" ]]; then
      echo "== CPU flamegraph (5k skip-heavy diff) =="
      # Repository-relative: absolute machine paths are useless to reviewers.
      echo "  ops/perf/artifacts/$svg"
      echo ""
      break
    fi
  done

  for f in plan_diff_dhat.txt plan_diff_dhat_transitions.txt plan_diff_dhat_scan.txt \
           rust_plan_diff_dhat.txt rust_plan_diff_dhat_transitions.txt rust_plan_diff_dhat_scan.txt; do
    if [[ -f "$ART/$f" ]]; then
      echo "== dhat summary: $f =="
      head -n 40 "$ART/$f"
      echo ""
    fi
  done

  for f in alloc_flame.txt alloc_flame_transitions.txt alloc_flame_scan.txt \
           rust_alloc_flame.txt rust_alloc_flame_transitions.txt rust_alloc_flame_scan.txt; do
    if [[ -f "$ART/$f" ]]; then
      echo "== alloc tree: $f =="
      head -n 30 "$ART/$f"
      echo ""
    fi
  done

  for f in footprint_bench.txt rust_footprint_bench.txt; do
    if [[ -f "$ART/$f" ]]; then
      echo "== criterion bench log (tail) =="
      tail -n 15 "$ART/$f"
      echo ""
      break
    fi
  done
} | tee "$OUT"

echo "Wrote $OUT"
