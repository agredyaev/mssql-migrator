#!/usr/bin/env bash
# In-process footprint: struct sizes, plan diff 5k bench, CPU flamegraph, dhat alloc.
# Harness crate: migrator-core-dev (not linked from rmig/rmigd production binaries).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
ARTIFACTS="$ROOT/ops/perf/artifacts"
PKG=migrator-core-dev
mkdir -p "$ARTIFACTS"

MODE="${1:-bench}"
shift || true

feat_for_alloc() {
  case "${1:-skip_heavy}" in
    skip_heavy|"") echo "bench-skip" ;;
    transitions)   echo "bench-transitions" ;;
    scan)          echo "bench-scan" ;;
    scan_root)     echo "bench-scan" ;;
    cache)         echo "bench-skip" ;;
    *) echo "unknown alloc bench: $1 (skip_heavy|transitions|scan|scan_root|cache)" >&2; exit 2 ;;
  esac
}

case "$MODE" in
  bench)
    RMIG_FOOTPRINT_REPORT=1 \
      cargo test -p "$PKG" --test footprint_baseline -- --nocapture
    cargo bench -p "$PKG" --bench plan_diff --features bench-skip -- --noplot 2>&1 | tee "$ARTIFACTS/footprint_bench.txt"
    cargo test -p "$PKG" --test footprint_baseline footprint_baseline_match -q
    ;;
  profile)
    cargo bench -p "$PKG" --bench plan_diff --features bench-skip -- --profile-time=5 "$@"
    FG="$(find target/criterion -name 'flamegraph.svg' 2>/dev/null | head -1 || true)"
    if [ -n "$FG" ]; then
      cp -f "$FG" "$ARTIFACTS/plan_diff_5k_flamegraph.svg"
      echo "CPU flamegraph: $ARTIFACTS/plan_diff_5k_flamegraph.svg (source: $FG)"
    else
      echo "warning: flamegraph.svg not found under target/criterion (RmigPprofProfiler / --profile-time)" >&2
    fi
    "$ROOT/ops/perf/profile_summary.sh" 2>/dev/null || true
    ;;
  profile-load)
    RMIG_REPO_ROOT="$ROOT" \
    RMIG_PROFILE_SECS="${RMIG_PROFILE_SECS:-30}" \
    RMIG_PPROF_FREQ="${RMIG_PPROF_FREQ:-1000}" \
      cargo bench -p "$PKG" --bench plan_diff_load --features bench-skip --profile profiling 2>&1 \
      | tee "$ARTIFACTS/plan_diff_load_run.txt"
    echo "CPU flamegraph: $ARTIFACTS/plan_diff_5k_load_flamegraph.svg"
    echo "text summary:  $ARTIFACTS/plan_diff_load_profile.txt"
    ;;
  profile-load-transitions)
    RMIG_REPO_ROOT="$ROOT" \
    RMIG_PROFILE_SECS="${RMIG_PROFILE_SECS:-30}" \
    RMIG_PPROF_FREQ="${RMIG_PPROF_FREQ:-1000}" \
      cargo bench -p "$PKG" --bench plan_diff_load_transitions --features bench-transitions --profile profiling 2>&1 \
      | tee "$ARTIFACTS/plan_diff_transitions_load_run.txt"
    echo "CPU flamegraph: $ARTIFACTS/plan_diff_transitions_load_flamegraph.svg"
    echo "text summary:  $ARTIFACTS/plan_diff_transitions_load_profile.txt"
    ;;
  profile-load-scan)
    RMIG_REPO_ROOT="$ROOT" \
    RMIG_PROFILE_SECS="${RMIG_PROFILE_SECS:-30}" \
    RMIG_PPROF_FREQ="${RMIG_PPROF_FREQ:-1000}" \
      cargo bench -p "$PKG" --bench scan_load --features bench-scan --profile profiling 2>&1 \
      | tee "$ARTIFACTS/scan_load_run.txt"
    echo "CPU flamegraph: $ARTIFACTS/scan_5k_load_flamegraph.svg"
    echo "text summary:  $ARTIFACTS/scan_load_profile.txt"
    ;;
  profile-load-cache)
    RMIG_REPO_ROOT="$ROOT" \
    RMIG_PROFILE_SECS="${RMIG_PROFILE_SECS:-30}" \
    RMIG_PPROF_FREQ="${RMIG_PPROF_FREQ:-1000}" \
      cargo bench -p "$PKG" --bench cache_serde_load --features bench-skip --profile profiling 2>&1 \
      | tee "$ARTIFACTS/cache_serde_load_run.txt"
    echo "CPU flamegraph: $ARTIFACTS/cache_serde_load_flamegraph.svg"
    echo "text summary:  $ARTIFACTS/cache_serde_load_profile.txt"
    ;;
  alloc)
    BENCH="${1:-skip_heavy}"
    shift || true
    FEAT="$(feat_for_alloc "$BENCH")"
    case "$BENCH" in
      skip_heavy|"") DHAT_BENCH=plan_diff_dhat; DHAT_OUT=plan_diff_dhat.txt ;;
      transitions)   DHAT_BENCH=plan_diff_dhat_transitions; DHAT_OUT=plan_diff_dhat_transitions.txt ;;
      scan)          DHAT_BENCH=plan_diff_dhat_scan; DHAT_OUT=plan_diff_dhat_scan.txt ;;
      scan_root)     DHAT_BENCH=scan_dhat; DHAT_OUT=scan_dhat.txt ;;
      cache)         DHAT_BENCH=cache_serde_dhat; DHAT_OUT=cache_serde_dhat.txt ;;
    esac
    cargo bench -p "$PKG" --bench "$DHAT_BENCH" --features "$FEAT" --profile profiling 2>&1 | tee "$ARTIFACTS/$DHAT_OUT"
    if [ -f dhat-heap.json ]; then
      cp -f dhat-heap.json "$ARTIFACTS/dhat_heap.json"
    elif [ -f "$ROOT/crates/core-dev/dhat-heap.json" ]; then
      cp -f "$ROOT/crates/core-dev/dhat-heap.json" "$ARTIFACTS/dhat_heap.json"
    fi
    if [ -f "$ARTIFACTS/dhat_heap.json" ]; then
      FLAME="$ARTIFACTS/alloc_flame.txt"
      case "$BENCH" in
        transitions) FLAME="$ARTIFACTS/alloc_flame_transitions.txt" ;;
        scan)        FLAME="$ARTIFACTS/alloc_flame_scan.txt" ;;
        scan_root)   FLAME="$ARTIFACTS/alloc_flame_scan_root.txt" ;;
        cache)       FLAME="$ARTIFACTS/alloc_flame_cache.txt" ;;
      esac
      python3 "$ROOT/ops/perf/dhat_alloc_tree.py" "$ARTIFACTS/dhat_heap.json" --iterations 20 \
        | tee "$FLAME"
    fi
    echo "dhat report: $ARTIFACTS/$DHAT_OUT"
    echo "alloc tree:  $FLAME"
    ;;
  update-baseline)
    RMIG_FOOTPRINT_UPDATE_BASELINE=1 \
      cargo test -p "$PKG" --test footprint_baseline update_footprint_baseline -- --nocapture
    cp -f "$ROOT/crates/core/tests/testdata/perf/footprint_baseline.json" "$ARTIFACTS/footprint_baseline.json" 2>/dev/null || true
    echo "Baseline: crates/core/tests/testdata/perf/footprint_baseline.json"
    ;;
  regression)
    cargo test -p "$PKG" --test footprint_baseline footprint_baseline_match -v -- --nocapture
    ;;
  *)
    echo "usage: $0 {bench|profile|profile-load|profile-load-scan|profile-load-cache|alloc|update-baseline|regression} [args...]" >&2
    echo "  profile-load  RMIG_PROFILE_SECS=30 RMIG_PPROF_FREQ=1000 (sustained compute_diff_into loop)" >&2
    echo "  profile-load-scan / profile-load-cache  (sustained scan_root / L1 serde loop)" >&2
    echo "  alloc [skip_heavy|transitions|scan|scan_root|cache]" >&2
    exit 2
    ;;
esac
