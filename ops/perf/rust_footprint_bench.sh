#!/usr/bin/env bash
# Rust in-process footprint: struct sizes, plan diff 5k bench, CPU flamegraph, dhat alloc.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT/rust"
ARTIFACTS="$ROOT/ops/perf/artifacts"
mkdir -p "$ARTIFACTS"

MODE="${1:-bench}"
shift || true

case "$MODE" in
  bench)
    RMIG_RUST_FOOTPRINT_REPORT=1 \
      cargo test -p migrator-core --test rust_footprint_baseline -- --nocapture
    cargo bench -p migrator-core --bench plan_diff -- --noplot 2>&1 | tee "$ARTIFACTS/rust_footprint_bench.txt"
    cargo test -p migrator-core --test rust_footprint_baseline footprint_baseline_match -q
    ;;
  profile)
    cargo bench -p migrator-core --bench plan_diff -- --profile-time=5 "$@"
    FG="$(find target/criterion -name 'flamegraph.svg' 2>/dev/null | head -1 || true)"
    if [ -n "$FG" ]; then
      cp -f "$FG" "$ARTIFACTS/rust_plan_diff_5k_flamegraph.svg"
      echo "CPU flamegraph: $ARTIFACTS/rust_plan_diff_5k_flamegraph.svg (source: $FG)"
    else
      echo "warning: flamegraph.svg not found under target/criterion (RmigPprofProfiler / --profile-time)" >&2
    fi
    "$ROOT/ops/perf/profile_summary.sh" 2>/dev/null || true
    ;;
  alloc)
    BENCH="${1:-skip_heavy}"
    shift || true
    case "$BENCH" in
      skip_heavy|"") DHAT_BENCH=plan_diff_dhat; DHAT_OUT=rust_plan_diff_dhat.txt ;;
      transitions)   DHAT_BENCH=plan_diff_dhat_transitions; DHAT_OUT=rust_plan_diff_dhat_transitions.txt ;;
      scan)          DHAT_BENCH=plan_diff_dhat_scan; DHAT_OUT=rust_plan_diff_dhat_scan.txt ;;
      *) echo "unknown alloc bench: $BENCH (skip_heavy|transitions|scan)" >&2; exit 2 ;;
    esac
    cargo bench -p migrator-core --bench "$DHAT_BENCH" --profile profiling 2>&1 | tee "$ARTIFACTS/$DHAT_OUT"
    if [ -f dhat-heap.json ]; then
      cp -f dhat-heap.json "$ARTIFACTS/rust_dhat_heap.json"
    elif [ -f "$ROOT/rust/crates/core/dhat-heap.json" ]; then
      cp -f "$ROOT/rust/crates/core/dhat-heap.json" "$ARTIFACTS/rust_dhat_heap.json"
    fi
    if [ -f "$ARTIFACTS/rust_dhat_heap.json" ]; then
      FLAME="$ARTIFACTS/rust_alloc_flame.txt"
      if [ "$BENCH" = "transitions" ]; then
        FLAME="$ARTIFACTS/rust_alloc_flame_transitions.txt"
      elif [ "$BENCH" = "scan" ]; then
        FLAME="$ARTIFACTS/rust_alloc_flame_scan.txt"
      fi
      python3 "$ROOT/ops/perf/dhat_alloc_tree.py" "$ARTIFACTS/rust_dhat_heap.json" --iterations 20 \
        | tee "$FLAME"
    fi
    echo "dhat report: $ARTIFACTS/$DHAT_OUT"
    echo "alloc tree:  $FLAME"
    ;;
  update-baseline)
    RMIG_RUST_FOOTPRINT_UPDATE_BASELINE=1 \
      cargo test -p migrator-core --test rust_footprint_baseline update_footprint_baseline -- --nocapture
    cp -f "$ROOT/internal/app/testdata/perf/rust_footprint_baseline.json" "$ARTIFACTS/rust_footprint_baseline.json" 2>/dev/null || true
    echo "Baseline: internal/app/testdata/perf/rust_footprint_baseline.json"
    ;;
  regression)
    cargo test -p migrator-core --test rust_footprint_baseline footprint_baseline_match -v -- --nocapture
    ;;
  *)
    echo "usage: $0 {bench|profile|alloc|update-baseline|regression} [args...]" >&2
    echo "  alloc [skip_heavy|transitions|scan]" >&2
    exit 2
    ;;
esac
