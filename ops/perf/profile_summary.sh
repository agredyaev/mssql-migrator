#!/usr/bin/env bash
# Summarize committed perf artifacts (regenerate with make bench-footprint* first).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ART="${RMIG_FOOTPRINT_ARTIFACTS:-$ROOT/ops/perf/artifacts}"
OUT="${ART}/profile_summary.txt"
mkdir -p "$ART"
# shellcheck source=ops/perf/profile_identity.sh
source "$ROOT/ops/perf/profile_identity.sh"
PROFILE_ID="$(rmig_profile_identity "$ROOT")"

require_profile_identity() {
  local file="$1"
  if [[ "$(head -n 1 "$file")" != "# $PROFILE_ID" ]]; then
    echo "ERROR: stale or foreign profile artifact: $file" >&2
    exit 1
  fi
}

{
  echo "# rmig profile summary - $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "# $PROFILE_ID"
  echo "# Regenerate: make bench-footprint bench-footprint-profile bench-footprint-alloc"
  echo "# Then: ops/perf/footprint_bench.sh alloc transitions; ops/perf/footprint_bench.sh alloc scan"
  echo ""

  for svg in rust_plan_diff_5k_flamegraph.svg; do
    if [[ -f "$ART/$svg" ]]; then
      if [[ "$(tail -n 1 "$ART/$svg")" != "<!-- $PROFILE_ID -->" ]]; then
        echo "ERROR: stale or missing flamegraph identity: $ART/$svg" >&2
        exit 1
      fi
      echo "== CPU flamegraph (5k skip-heavy diff) =="
      # Repository-relative: absolute machine paths are useless to reviewers.
      echo "  ops/perf/artifacts/$svg"
      echo ""
      break
    fi
  done

  for f in rust_plan_diff_dhat.txt rust_plan_diff_dhat_transitions.txt rust_plan_diff_dhat_scan.txt; do
    if [[ -f "$ART/$f" ]]; then
      require_profile_identity "$ART/$f"
      echo "== dhat summary: $f =="
      head -n 40 "$ART/$f" | sed "s|${ROOT}/||g"
      echo ""
    fi
  done

  for f in rust_alloc_flame.txt rust_alloc_flame_transitions.txt rust_alloc_flame_scan.txt; do
    if [[ -f "$ART/$f" ]]; then
      require_profile_identity "$ART/$f"
      echo "== alloc tree: $f =="
      head -n 30 "$ART/$f" | sed "s|${ROOT}/||g"
      echo ""
    fi
  done

  for f in rust_footprint_bench.txt; do
    if [[ -f "$ART/$f" ]]; then
      require_profile_identity "$ART/$f"
      echo "== criterion bench log (tail) =="
      tail -n 15 "$ART/$f" | sed "s|${ROOT}/||g"
      echo ""
      break
    fi
  done
} | tee "$OUT"

echo "Wrote $OUT"
