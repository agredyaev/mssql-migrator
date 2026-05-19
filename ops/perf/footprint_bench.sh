#!/usr/bin/env bash
# Capture in-process footprint baseline (struct sizes + diff benches). No SQL Server required.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

ARTIFACTS="${RMIG_FOOTPRINT_ARTIFACTS:-$ROOT/ops/perf/artifacts}"
mkdir -p "$ARTIFACTS"

MODE="${1:-bench}"
shift || true

case "$MODE" in
  bench)
    go test ./internal/perf/ -run=NONE \
      -bench='BenchmarkDiffCompute_SkipHeavy_' \
      -benchmem -count=5 "$@" | tee "$ARTIFACTS/footprint_bench.txt"
    go test ./internal/perf/ -run TestStructSizeReport -v -count=1 "$@"
    ;;
  profile)
    go test ./internal/perf/ -run=NONE \
      -bench=BenchmarkDiffCompute_SkipHeavy_5000Objects \
      -benchmem -count=1 \
      -cpuprofile="$ARTIFACTS/footprint_5k.cpu.prof" \
      -memprofile="$ARTIFACTS/footprint_5k.mem.prof" \
      -memprofilerate=1 \
      "$@"
    echo "CPU:  $ARTIFACTS/footprint_5k.cpu.prof  (go tool pprof -http=:0 $ARTIFACTS/footprint_5k.cpu.prof)"
    echo "Mem:  $ARTIFACTS/footprint_5k.mem.prof  (go tool pprof -http=:0 $ARTIFACTS/footprint_5k.mem.prof)"
    "$(dirname "$0")/profile_summary.sh" 2>/dev/null || true
    ;;
  update-baseline)
    export RMIG_FOOTPRINT_UPDATE_BASELINE=1
    export RMIG_FOOTPRINT_ARTIFACTS="$ARTIFACTS"
    go test ./internal/perf/ -run TestUpdateFootprintBaseline -v -count=1 "$@"
    cp -f internal/app/testdata/perf/footprint_baseline.json "$ARTIFACTS/footprint_baseline.json" 2>/dev/null || true
    echo "Baseline: internal/app/testdata/perf/footprint_baseline.json"
    ;;
  regression)
    export RMIG_FOOTPRINT_BENCH=1
    go test ./internal/perf/ -run 'TestFootprintBaselineMatch|TestFootprintBenchmarkRegression' -v -count=1 "$@"
    ;;
  *)
    echo "usage: $0 {bench|profile|update-baseline|regression} [go test args...]" >&2
    exit 2
    ;;
esac
