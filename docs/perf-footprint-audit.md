# Footprint and allocation audit

Lifecycle: `Current`.

## Purpose

Measure in-process memory and CPU of the Rust plan pipeline (scan, diff, plan output). Layout rules: [`data-oriented-layout-policy.md`](data-oriented-layout-policy.md).

## Scope

- Harness: [`crates/core-dev/`](../crates/core-dev/) (`migrator-core-dev`, not a production dependency)
- Scripts: [`ops/perf/footprint_bench.sh`](../ops/perf/footprint_bench.sh), [`ops/perf/dhat_alloc_tree.py`](../ops/perf/dhat_alloc_tree.py)
- Baseline: [`crates/core/tests/testdata/perf/footprint_baseline.json`](../crates/core/tests/testdata/perf/footprint_baseline.json)
- Bench: `plan_diff_skip_heavy_5000` in [`crates/core-dev/benches/plan_diff.rs`](../crates/core-dev/benches/plan_diff.rs)
- Phase profilers: [`scan_load.rs`](../crates/core-dev/benches/scan_load.rs) /
  [`scan_dhat.rs`](../crates/core-dev/benches/scan_dhat.rs) (filesystem scan).
  These start the profiler after warmup, so dhat **Total** ÷ 20 is the
  per-iteration cost.

**Out of scope:** SQL Server tuning, `cli_wall_ms` SLO (`make slo`), CI hard perf gates.

## System context

Footprint work validates layout policy under [`data-oriented-layout-policy.md`](data-oriented-layout-policy.md). Production binaries do not link `migrator-core-dev`; results are advisory unless a maintainer updates the committed baseline JSON.

## Interfaces and boundaries

| Input | Output |
|-------|--------|
| `make bench-footprint` | `artifacts/footprint_bench.txt`, struct size JSON |
| `make bench-footprint-alloc` | `artifacts/rust_plan_diff_dhat.txt`, `artifacts/rust_alloc_flame.txt` |
| `footprint_baseline_match` test | pass/fail vs committed JSON |

## Assumptions and constraints

- Skip-heavy 5k workspace is the headline diff benchmark.
- dhat **loop** phase B/iter is the allocation regression signal.
- Struct sizes are platform-dependent; the committed baseline is validated by `make bench-footprint` (which runs `footprint_baseline_match` in `migrator-core-dev`); `make check` does not select that crate.
- Plan cost and footprint are O(catalog size). The owned object layout
  intentionally trades smaller structs for simpler exact-key state. Do not
  infer runtime cost from `size_of`; use criterion, DHAT, and the live SLO
  commands below.

## Nominal flow

1. `make bench-footprint` — struct sizes + criterion + baseline match.
2. Optional: `make bench-footprint-profile` (CPU), `make bench-footprint-alloc` (heap).
3. On intentional size change: `make bench-footprint-update-baseline` and commit JSON.

## Off-nominal behavior

- Baseline mismatch without JSON update → test failure; do not silence without review.
- Large loop-phase allocations → layout regression; compare `alloc_flame.txt` to prior artifact.

## Verification

```bash
make bench-footprint                    # struct sizes + criterion bench + baseline match
make bench-footprint-profile            # CPU flamegraph (5k skip-heavy diff)
make bench-footprint-alloc              # dhat + alloc call-tree (default: skip_heavy)
make bench-footprint-alloc ARGS=transitions
make bench-footprint-alloc ARGS=scan
ops/perf/footprint_bench.sh profile-load-scan    # CPU flamegraph: scan_root (5k files)
ops/perf/footprint_bench.sh alloc scan_root      # dhat: scan_root loop-only
make bench-footprint-update-baseline    # maintainer: refresh committed JSON
make profile-summary                    # text rollup of artifacts/
cargo test -p migrator-core-dev --test footprint_baseline footprint_baseline_match -q
```

## Operations and recovery

| Artifact | Meaning |
|----------|---------|
| `artifacts/struct_sizes.json` | `sizeof` snapshot |
| `artifacts/footprint_bench.txt` | criterion log |
| `artifacts/rust_plan_diff_5k_flamegraph.svg` | CPU hot path |
| `artifacts/rust_plan_diff_dhat.txt` | dhat summary (skip-heavy) |
| `artifacts/dhat_heap.json` | raw dhat heap (input to Python tree) |
| `artifacts/alloc_flame.txt` | human alloc tree from `dhat_alloc_tree.py` |
| `artifacts/scan_5k_load_flamegraph.svg`, `artifacts/scan_dhat.txt` | scan_root CPU + heap |

dhat phases (`dhat_alloc_tree.py`):

| Phase | Meaning |
|-------|---------|
| setup | fixture build (`skip_heavy_workspace`, scan ingest) |
| warm | first `compute_diff_into` (plan Vec resize) |
| loop | warmed iterations (headline **B/iter**) |

Recovery: re-run full bench suite after layout PR; attach artifacts to the PR when sizes or loop alloc change.

Large catalogs: for a full plan over tens of thousands of objects, the checksum/drift query can approach the default 30 s `RM_COMMAND_TIMEOUT` and fail with `query timed out after 30s` (exit 5). Raise `RM_COMMAND_TIMEOUT` (seconds) for such catalogs. The measured in-memory and live footprint curve (2k/20k/100k) is reproducible via `crates/core-dev/tests/scale_footprint.rs` (set `RMIG_SCALE_N`, run under `/usr/bin/time -l`).

## Open issues and non-goals

- Non-goals: gating merges on criterion wall time; substituting footprint for `make slo`.

## References

- [`ops/perf/README.md`](../ops/perf/README.md)
- [`docs/data-oriented-layout-policy.md`](data-oriented-layout-policy.md)
- [`docs/specs/rust/module-domain.md`](specs/rust/module-domain.md)
