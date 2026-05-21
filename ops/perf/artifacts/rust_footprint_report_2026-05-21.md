# Rust footprint audit — full run

Lifecycle: snapshot.

**Environment:** darwin/arm64, local `cargo bench` / `cargo test`.  
**Harness:** [`ops/perf/rust_footprint_bench.sh`](../../rust_footprint_bench.sh), runbook [`docs/perf-footprint-audit.md`](../../../docs/perf-footprint-audit.md).

## Commands (Rust only)

```bash
make rust-bench-footprint
make rust-bench-footprint-profile
make rust-bench-footprint-alloc
make rust-bench-footprint-alloc ARGS=transitions
make rust-bench-footprint-alloc ARGS=scan
cargo test -p migrator-core --test rust_footprint_baseline footprint_baseline_match -q
```

## Struct sizes (`size_of`, threshold ≥ 40 B)

| package | type | bytes |
|---------|------|------:|
| domain | WorkspaceCold | 736 |
| export | MigrationPlan | 328 |
| config | ConfigCold | 232 |
| timings | PhaseTimings | 168 |
| config | Config | 144 |
| export | PlannedObject | 144 |
| gate | PlanSnapshot | 104 |
| domain | Workspace | 88 |
| gate | SnapshotObject | 80 |
| domain | ObjectStore | 72 |
| domain | ObjectEntry | 56 |
| export | PlanSummary | 56 |
| domain | ScriptRow | 40 |

Hot layout: `ObjectEntry` 56 B (`key_off` @0), `Workspace` 88 B, `ObjectRow` 4 B.  
Baseline JSON: `internal/app/testdata/perf/rust_footprint_baseline.json` — **match OK**.

Full JSON: `ops/perf/artifacts/rust_struct_sizes.json`.

## CPU — `plan_diff_skip_heavy_5000` (criterion, warmed `compute_diff_into`)

| metric | value |
|--------|------:|
| time | **177.46–177.70 µs/iter** (p50 ~177.6 µs) |

Log: `ops/perf/artifacts/rust_footprint_bench.txt`.

## CPU flamegraph (5 s profile)

| frame (cum samples) | share |
|---------------------|------:|
| `compute_diff_into` | ~119% * |
| `fill_prior_by_row` | ~58% |
| `row_id_for_fingerprint` | ~58% |

\* Criterion stack overlap; interpret relatively.

SVG: `ops/perf/artifacts/rust_plan_diff_5k_flamegraph.svg`.

## Heap — dhat (20× warmed `compute_diff_into` per bench)

| bench | setup | warm | **loop B/iter** |
|-------|------:|-----:|----------------:|
| skip_heavy (5k) | 12.79 MB | 335.1 KB | **0** |
| transitions | 2.72 MB | 33.6 KB | **0** |
| scan (fixture) | 13.60 MB | 335.1 KB | **0** |

Reports:

- `rust_plan_diff_dhat.txt`
- `rust_plan_diff_dhat_transitions.txt`
- `rust_plan_diff_dhat_scan.txt`

Alloc trees: `rust_alloc_flame.txt`, `rust_alloc_flame_transitions.txt`, `rust_alloc_flame_scan.txt`.  
Raw: `rust_dhat_heap.json` (skip_heavy).

## Invariants (this run)

- Loop-phase alloc **0 B/iter** on all three dhat benches.
- `fill_prior_by_row`: O(|checksums|) via `fp_index`, no per-row `key_fingerprint`.
- `SharedStr::Hash` / `ChecksumMap` use byte fingerprints (no `as_str` on hot hash).

## Artifacts index

| artifact | path |
|----------|------|
| struct JSON | `ops/perf/artifacts/rust_struct_sizes.json` |
| criterion log | `ops/perf/artifacts/rust_footprint_bench.txt` |
| CPU SVG | `ops/perf/artifacts/rust_plan_diff_5k_flamegraph.svg` |
| dhat ×3 | `ops/perf/artifacts/rust_plan_diff_dhat*.txt` |
| alloc trees ×3 | `ops/perf/artifacts/rust_alloc_flame*.txt` |
