# Technical Document: Internal Performance Audit

Lifecycle: `Current`.

## Purpose

This document validates the current `internal/` implementation against the data-structure and algorithm guidance at [Data Structures Optimization](https://psavelis.github.io/golang-performance-optimization/optimization/algorithms/data-structures.html).
It records which performance claims are supported by the code, which claims are outdated or unsupported, and which suspected hotspots still require benchmarks.

**Where to see before/after numbers:** start at `docs/perf/README.md` (index) and `docs/perf/runs/2026-05-18-baseline-vs-post/README.md` (TL;DR table — memory and allocs improved on apply benches; `ExecuteTxBatch` `ns/op` is noise at `n=5`).

## Scope

- Runtime orchestration: `internal/engine/engine.go`
- Repository layout and file caching: `internal/fs/layout.go`, `internal/fs/scanner.go`, `internal/fs/normalize.go`
- Scanner benchmark harness: `internal/fs/scanner_bench_test.go`, `internal/fs/layout_bench_test.go`, `docs/perf/scanner-preload-gitinfo-plan.md`
- Planning logic: `internal/diff/diff.go`, `internal/diff/diff_bench_test.go`
- Apply path: `internal/apply/apply.go`, `internal/apply/apply_bench_test.go`
- Event delivery: `internal/bus/bus.go`, `internal/bus/bus_bench_test.go`
- Catalog inspection and query shaping: `internal/db/inspector_impl.go`, `internal/db/inspector_bench_test.go`, `internal/types/chunk.go`
- Existing benchmark and profiling evidence: `internal/engine/benchmark_profiling_test.go`, `internal/diff/diff_bench_test.go`, `internal/types/chunk_bench_test.go`, `internal/bus/bus_bench_test.go`, `internal/db/inspector_bench_test.go`, `internal/apply/apply_bench_test.go`, `docs/perf/scopekey-optimization-plan.md`, `docs/perf/scanner-preload-gitinfo-plan.md`, `docs/perf/runs/2026-05-18-scanner-preload-gitinfo-profile/README.md`, `Makefile`, `docs/profiling-benchmark-plan.md`, `docs/perf/runs/2026-05-17-profiling-round/TRIAGE.md`, `docs/perf/runs/2026-05-18-audit-bench/bench-output.txt`, `docs/perf/runs/2026-05-18-audit-bench/benchstat-before-add-apply-benches.txt`, `docs/perf/runs/2026-05-18-audit-bench/benchstat-first-vs-latest.txt`, `docs/perf/runs/2026-05-18-apply-fs-profile/README.md`, `docs/perf/runs/2026-05-18-baseline-vs-post/README.md`

## System Context

The main hot path for a normal repository-driven run is:

1. `(*Engine).runPlan()` in `internal/engine/engine.go`
2. `(*Scanner).Scan()` in `internal/fs/scanner.go`
3. `preloadGitInfo()` and `preloadChecksums()` in `internal/fs/scanner.go`
4. `(*inspector).Inspect()` in `internal/db/inspector_impl.go`
5. `(*Computer).Compute()` in `internal/diff/diff.go`
6. `(*Engine).filterAppliedMigrations()` in `internal/engine/engine.go`
7. `(*Executor).Execute()` in `internal/apply/apply.go`
8. `Bus.Publish()` in `internal/bus/bus.go`

The external article is useful as a decision checklist: choose data structures from access patterns, pre-allocate when size is known, use pooling only for expensive objects, and confirm wins with measurement. This audit applies that checklist to the current code instead of assuming that every micro-optimization idea is worth implementing.

## Interfaces And Boundaries

- Inputs: repository SQL files under the configured `SQLRoot`, database metadata returned through `internal/driver.Conn`, checksum history returned by `Loader.LoadChecksums()`, and benchmark evidence from `go test -bench`
- Outputs: a validated classification of performance claims, a list of benchmark targets, and exact repository paths that own each finding
- Ownership boundaries: this audit covers `internal/` code shape and checked-in benchmark harnesses only; it does not prove production SQL Server latency, OS scheduler behavior, or network round-trip costs

## Assumptions And Constraints

- Assumptions:
  - The current repository state is the source of truth for this audit.
  - `internal/engine/benchmark_profiling_test.go` and `internal/types/chunk_bench_test.go` are the canonical local measurement harnesses for the covered paths.
  - The same `inspector` instance is normally used within one engine run and one target connection.
- Constraints:
  - Code inspection can prove data structure choice, allocation shape, and concurrency semantics, but it cannot prove wall-clock benefit without benchmark evidence.
  - Existing benchmarks build synthetic repositories and temporary Git history. They are useful for relative changes inside `internal/`, but they are not a substitute for live MSSQL workload profiling.
  - The article discusses wider options such as ring buffers, tries, skip lists, and structure-of-arrays. Those structures are not automatically relevant to this repository because the current hot paths are dominated by strings, maps, file reads, hashing, and SQL batching.

## Nominal Flow

1. `internal/fs/scanner.go` walks the repository tree with `os.ReadDir`, constructs `fs.Layout`, sorts transition scripts by ordinal, preloads Git metadata, and preloads checksums.
2. `internal/db/inspector_impl.go` derives schema and object scope from `fs.Layout`, deduplicates names with maps, and chunks `IN` query arguments with `types.ChunkKeys`.
3. `internal/diff/diff.go` pre-allocates the main plan slices, optionally builds `transitionsByKey`, computes object actions from map lookups and cached checksums (one **`Checksum()`** read per object), and on cold checksum caches runs **`warmupAll`** with a bounded worker pool before the main object loop.
4. `internal/engine/engine.go` filters already-applied transition paths with one map lookup per transition after an early `needLookup` check.
5. `internal/apply/apply.go` builds path indexes once, groups transactional statements into batches, sorts each batch by object kind, and builds batch SQL with `strings.Builder`.
6. `internal/bus/bus.go` publishes events synchronously to the current subscriber slice, invoking each handler under `recover` via `invokeBusHandler`.

## Findings

### Confirmed Optimizations

| Area | Evidence | Validation |
| --- | --- | --- |
| Pre-allocation on planner and lookup paths | `internal/diff/diff.go` pre-allocates `plan.Schemas`, `plan.Objects`, and lazy `plan.Blockers`; `internal/fs/layout.go` keeps `ObjectsByPath` / `TransitionsByPath` maps on `Layout` (built at end of `Scanner.Scan`, or lazily on first use, or via `RebuildPathIndexes` after mutating slices) | Confirmed by code |
| Reuse of expensive byte buffers | `internal/fs/normalize.go` uses `normalizePool`; `internal/fs/layout.go` uses `layoutHashDigestsPool` for `[][32]byte` digest storage | Confirmed by code |
| Transition scaffold detection avoids full file reads | `parseTransitionFile()` in `internal/fs/scanner.go` reads only the prefix needed to classify the first line instead of loading the entire transition file | Confirmed by code and local scanner benchmark |
| Lazy file and Git metadata caching | `CachedFile` in `internal/fs/layout.go` uses `sync.Once` for content, checksum, and Git metadata | Confirmed by code |
| Chunked query shaping instead of oversized `IN (...)` lists | `internal/types/chunk.go` and `internal/db/inspector_impl.go` chunk schema and object lists by `driver.DefaultMaxParameters` | Confirmed by code |
| Warm-path avoidance of goroutine fan-out in planner | `warmupIfNeeded()` in `internal/diff/diff.go` skips `warmupAll()` when checksums are already cached by `Scanner.preloadChecksums()`; cold `warmupAll()` uses a **bounded worker pool** (`min(GOMAXPROCS(0), N)`) over a jobs channel instead of **N** goroutines plus a semaphore | Confirmed by code, `BenchmarkWarmupAll_500ColdObjects` in `internal/diff/diff_bench_test.go`, and existing benchmark narrative in `docs/profiling-benchmark-plan.md` |
| Batch SQL construction avoids repeated string concatenation | `buildTxBatchSQL()` in `internal/apply/apply.go` already uses `strings.Builder` | Confirmed by code |
| Early exit before applied-transition lookup | `(*Engine).filterAppliedMigrations()` in `internal/engine/engine.go` avoids `LoadAllAppliedMigrations()` when no object is in the `ActionReprocessChanged` plus `TransitionPaths` state | Confirmed by code and tests |
| Bus handler dispatch | `(*Bus).Publish` in `internal/bus/bus.go` calls **`invokeBusHandler`** so each handler runs under **`recover`** without a fresh **anonymous function literal per subscriber** | Confirmed by code |
| Inspector cache key materialization | `scopeKey()` in `internal/db/inspector_impl.go` collects **`[]scopePart` `{kind byte, s string}`**, sorts with **`sort.Slice`** (tuple order matches legacy `"kind:"+s` lexicographic order), then emits once via **`strings.Builder`** — avoids **~n** small **`"x:"+payload` string allocations** and **`strings.Join`** | Confirmed by code, `TestScopeKeyGoldenMixed`, and `BenchmarkScopeKey_2000Parts` (`benchmem`) |

### Unsupported Or Outdated Claims

| Claim From Draft Analysis | Audit Verdict | Why |
| --- | --- | --- |
| `Bus.Publish()` has a memory leak and should move to buffered channels | Unsupported | `internal/bus/bus.go` shows synchronous fan-out over a handler slice. The proven issue is caller latency from slow handlers, not a memory leak. No channel-based queue exists in the current design. |
| `sort.Stable(kindSorter(...))` is a confirmed hot-path problem | Unsupported without profiling | The sort is real, but the repository already has no evidence that it dominates CPU or allocations. This is a benchmark target, not a validated bottleneck. |
| `internal/apply/apply.go` still needs a `strings.Builder` fix | Outdated | `buildTxBatchSQL()` already uses `strings.Builder`. |
| A pool for `kindSorter` objects would reduce allocations | Unsupported and low-value | `kindSorter` is a slice type alias over an existing `[]batchedStmt`. The sort wrapper does not allocate a separate sorter object to pool. |
| The semaphore in checksum and Git preload limits goroutine count to `GOMAXPROCS` | Incorrect | `internal/fs/scanner.go` `preloadChecksums` still creates **one goroutine per file**; the semaphore only caps how many run concurrently. **`warmupAll()`** in `internal/diff/diff.go` no longer follows that pattern: it uses **`min(GOMAXPROCS(0), N)`** long-lived workers over a channel instead of **N** per-object goroutines. |
| Event bus buffering is missing from a path that already claims buffered delivery | Outdated | `internal/bus/bus.go` does not implement buffered or asynchronous delivery today. Any analysis that assumes buffered delivery is reading behavior that does not exist. |

### Benchmark-Needed Hypotheses

| Hypothesis | Evidence In Code | Why Measurement Is Still Required |
| --- | --- | --- |
| `Scanner.preloadGitInfo()` may be expensive on a large repository | The fast path runs `git log --name-only --format=COMMIT|...` from the repository root in `internal/fs/scanner.go` | The code proves whole-repository history parsing, but not whether it is material on expected repository sizes |
| `Scanner.preloadChecksums()` may trade first-plan latency for higher upfront CPU and RSS | `internal/fs/scanner.go` eagerly calls `Checksum()` for every object, transition, and check script | The trade-off is architectural and intentional; the local benchmark now measures it directly, but the right setting still depends on expected repository shape |
| `LayoutHash()` sorting cost may matter for very large layouts | `internal/fs/layout.go` collects all digests and sorts them with `sort.Slice` before hashing | This is an `O(n log n)` step, but the current repository evidence does not show it as a dominant cost after fixture setup is separated |
| `Bus.Publish()` handler latency may become user-visible | `internal/bus/bus.go` runs handlers synchronously in the caller goroutine | The blocking behavior is proven; the practical cost depends on subscriber count and handler duration |
| `inspector.cache` may grow without bound or retain temporary errors | `internal/db/inspector_impl.go` stores one `cachedScope` per `scopeKey`, and `sync.Once` caches both success and failure | The semantic risk is clear, but production impact depends on scope churn and engine lifetime |
| Transaction batch fallback may do expensive retry work on failure | `executeTxBatch()` in `internal/apply/apply.go` retries each statement individually after a failed batch | This cost only occurs on the failure path, so the priority depends on error frequency in real runs |

## Benchmark Backlog

| Priority | Benchmark Target | Paths | Status |
| --- | --- | --- | --- |
| 1 | `BenchmarkBusPublish_*` (empty, sleep, panic) | `internal/bus/bus_bench_test.go` | Implemented; see `docs/perf/runs/2026-05-18-audit-bench/bench-output.txt` |
| 2 | `BenchmarkInspectorInspect_Cold_500Objects` and `BenchmarkInspectorInspect_HotCache_500Objects` | `internal/db/inspector_bench_test.go` | Implemented; same artifact |
| 3 | `BenchmarkScopeKey_2000Parts` and `BenchmarkBuildDualINQuery_500x500` | `internal/db/inspector_bench_test.go`, `internal/types/chunk_bench_test.go` | Implemented. `scopeKey` uses sorted `scopePart` rows + `strings.Builder` (see `docs/perf/scopekey-optimization-plan.md`). `types.ChunkKeys` + IN expansion as before |
| 4 | `BenchmarkScannerPreloadGitInfo_200Paths` and `BenchmarkScannerPreloadGitInfo_200Paths_5kExtraGitFiles` | `internal/fs/scanner_bench_test.go` | Implemented; requires `git` on `PATH`. Fast path: newline walk on raw `git log` bytes, `parseBatchedGitLogCommitLine`, `wantedRel` filter + in-place `\`→`/`, `applyPreloadedGitInfo` — see `docs/perf/scanner-preload-gitinfo-plan.md`. **`pprof`:** `docs/perf/runs/2026-05-18-scanner-preload-gitinfo-profile/README.md` |
| 5 | `BenchmarkCollectStatements_500Transactional`, `BenchmarkExecuteTxBatch_SuccessPath_100Statements`, `BenchmarkExecuteTxBatch_FailurePath_100Statements` | `internal/apply/apply_bench_test.go` | Implemented; `collectStatements` uses one warmup before `ResetTimer` so the timed loop measures cached file content + sort; failure path uses `MockConn{FailN: 1}`. Allocator pass (2026-05): capacity hints + single backing slice for transactional rows; pooled `strings.Builder` for batch and per-statement transaction SQL; `batchedStmt` holds `checksumSum [32]byte` with lazy `checksumHex` memo on first bus publish (`*batchedStmt` into `newObjectEvent` / `newFailureEvent`) so success-batch publish does not re-hex every statement |
| 6 | `BenchmarkWarmupAll_500ColdObjects` | `internal/diff/diff_bench_test.go` | Implemented; exercises `warmupAll` on a layout whose objects have cold checksum caches (no `Scanner.preloadChecksums`). Uses `testing.B.Loop` after fixture setup; measures worker-pool fan-out vs historical one-goroutine-per-object warmup |

## Off-Nominal Behavior And Failure Containment

- Failure mode: a `Bus.Publish()` subscriber blocks or does expensive work.
  Containment: the current design keeps behavior simple and deterministic, but the caller is blocked until the handler returns. This is a latency risk, not silent queue growth.
- Failure mode: the first `Inspect()` for a scope fails because the database is temporarily unavailable.
  Containment: `internal/db/inspector_impl.go` caches the first error through `sync.Once`; recovery requires a new `inspector` instance or a new `scopeKey`.
- Failure mode: cold layouts trigger large preload fan-out.
  Containment: `preloadChecksums()` in `internal/fs/scanner.go` still launches **one goroutine per file** capped by a semaphore. **`warmupAll()`** in `internal/diff/diff.go` uses **`min(GOMAXPROCS(0), N)`** worker goroutines and a **jobs channel** so cold `Compute` does not create **O(N)** short-lived goroutines for checksum/Git warmup.
- Failure mode: a transactional batch fails in `internal/apply/apply.go`.
  Containment: the code rolls back, then retries statements one by one to isolate the failing object. This contains blast radius for diagnosis at the cost of extra failure-path work.

## Verification And Validation

- Contracts and checks:
  - `go test ./internal/bus ./internal/db ./internal/diff ./internal/fs ./internal/types ./internal/engine ./internal/apply`
  - `make bench-perf-audit` (see `Makefile`)
  - `go test ./internal/types -run '^$' -bench '^BenchmarkChunkKeys_10k_2100$' -benchmem -count=1`
  - `go test ./internal/diff -run '^$' -bench '^BenchmarkWarmupAll_500ColdObjects$' -benchmem -count=1`
  - `go test ./internal/engine -run '^$' -bench '^BenchmarkDiffCompute_SkipHeavy_2000Objects$|^BenchmarkLayoutHash_2000Objects$|^BenchmarkNormalizeAndHash_LargeSQL$' -benchmem -count=1`
  - `go test ./internal/fs -run '^$' -bench '^BenchmarkScanner(Scan_TransitionHeavy|PreloadChecksums_2000Files)$' -benchmem -count=5 -benchtime=150ms`
- Evidence artifacts:
  - `internal/engine/benchmark_profiling_test.go`
  - `internal/diff/diff_bench_test.go`
  - `internal/bus/bus_bench_test.go`
  - `internal/db/inspector_bench_test.go`
  - `internal/fs/scanner_bench_test.go`
  - `internal/fs/layout_bench_test.go`
  - `internal/apply/apply_bench_test.go`
  - `internal/types/chunk_bench_test.go`
  - `docs/profiling-benchmark-plan.md`
  - `docs/perf/runs/2026-05-18-audit-bench/bench-output.txt`
  - `docs/perf/runs/2026-05-18-audit-bench/benchstat-before-add-apply-benches.txt`
  - `docs/perf/runs/2026-05-18-audit-bench/benchstat-first-vs-latest.txt` (historical two-capture check before `internal/apply` benches existed)
  - **Profiling (`pprof`) for apply + path indexes:** `docs/perf/runs/2026-05-18-apply-fs-profile/README.md` (commands, checked-in `*.prof` files, and `go tool pprof -text` extracts: `mem-allocobj-top-*.txt`, optional `cpu-topcum-*.txt`)
  - **Bench delta vs audit capture:** `docs/perf/runs/2026-05-18-baseline-vs-post/README.md` plus `benchstat-apply-only.txt` / `benchstat-audit-full.txt`
- Spot checks executed for this audit on `darwin/arm64` with `cpu: Apple M2`:
  - `go test ./internal/bus ./internal/db ./internal/diff ./internal/fs ./internal/types ./internal/engine ./internal/apply`
  - `go test ./internal/types -run '^$' -bench '^BenchmarkChunkKeys_10k_2100$' -benchmem -count=1` -> `670.8 ns/op`, `128 B/op`, `1 allocs/op`
  - `go test ./internal/diff -run '^$' -bench '^BenchmarkWarmupAll_500ColdObjects$' -benchmem -count=1` -> ~`110 µs/op`, ~`450 B/op`, ~`11 allocs/op` (layout reheated after first `warmupAll`; re-run for your machine)
  - `go test ./internal/engine -run '^$' -bench '^BenchmarkDiffCompute_SkipHeavy_2000Objects$|^BenchmarkLayoutHash_2000Objects$|^BenchmarkNormalizeAndHash_LargeSQL$' -benchmem -count=1`
    - `BenchmarkDiffCompute_SkipHeavy_2000Objects-8` -> `425872 ns/op`, `426736 B/op`, `3 allocs/op`
    - `BenchmarkLayoutHash_2000Objects-8` -> `614196 ns/op`, `231 B/op`, `5 allocs/op`
    - `BenchmarkNormalizeAndHash_LargeSQL-8` -> `6800102 ns/op`, `40220 B/op`, `0 allocs/op`
  - `go test ./internal/fs -run '^$' -bench '^BenchmarkScanner(Scan_TransitionHeavy|PreloadChecksums_2000Files)$' -benchmem -count=5 -benchtime=150ms`
    - `BenchmarkScannerScan_TransitionHeavy-8` improved from `119.7 ms/op` median to `111.5 ms/op` median, `B/op` dropped by `32.10%`, and `allocs/op` dropped by `8.90%` in local `benchstat`
    - `BenchmarkScannerPreloadChecksums_2000Files-8` stayed statistically flat in local `benchstat`
  - **Measured backlog benchmarks (2026-05-18)** — `darwin/arm64`, `cpu: Apple M2`, `go test` with `-benchmem -count=5 -benchtime=150ms` on `internal/bus`, `internal/db`, `internal/fs`, and `internal/apply`. Full stdout: `docs/perf/runs/2026-05-18-audit-bench/bench-output.txt`. Same run is reproduced by `make bench-perf-audit`.
    - `benchstat` vs the capture taken immediately before adding the `internal/apply` benchmarks: `docs/perf/runs/2026-05-18-audit-bench/benchstat-before-add-apply-benches.txt` (shared benchmarks should be noise-only at `n=5`).
    - `benchstat` between two earlier captures of the bus or db benches only: `docs/perf/runs/2026-05-18-audit-bench/benchstat-first-vs-latest.txt` (historical; same noise caveats).
    - `BenchmarkBusPublish_1Handler_Empty`: ~`27.4–28.9 ns/op`, `0 B/op`, `0 allocs/op`
    - `BenchmarkBusPublish_10Handlers_Empty`: ~`71.9–72.6 ns/op`, `0 B/op`, `0 allocs/op`
    - `BenchmarkBusPublish_100Handlers_Empty`: ~`585–594 ns/op`, `0 B/op`, `0 allocs/op`
    - `BenchmarkBusPublish_10Handlers_Sleep100ns`: ~`12.7–13.5 µs/op` (scheduler noise around `time.Sleep`)
    - `BenchmarkBusPublish_10Handlers_Panic`: ~`2.75–2.84 µs/op`, `0 B/op`, `0 allocs/op`
    - `BenchmarkScopeKey_2000Parts`: ~`127–158 µs/op`, ~`107 KiB/op`, ~`5 allocs/op` (`darwin/arm64` spot check after `scopePart` + `strings.Builder` emission — re-run `make bench-perf-audit` for your machine)
    - `BenchmarkBuildDualINQuery_500x500`: ~`47–50 µs/op`, ~`52–77 KiB/op` (depends on SQL template length), ~`1005 allocs/op` (`darwin/arm64` spot check after `strings.Builder` + `strconv.AppendInt` placeholder expansion—remaining allocs are mostly `string`→`any` boxing in the `args` slice; re-run `make bench-perf-audit` for your machine)
    - `BenchmarkInspectorInspect_Cold_500Objects`: ~`166–168 µs/op`, ~`162 KiB/op`, ~`1797 allocs/op`
    - `BenchmarkInspectorInspect_HotCache_500Objects`: ~`29.2–29.5 µs/op`, ~`37.8 KiB/op`, ~`503 allocs/op` (no additional `MockConn` queries after the initial cold `Inspect`)
    - `BenchmarkScannerPreloadGitInfo_200Paths`: ~`173–181 ms/op` per iteration when `b.N=1` at `-benchtime=150ms` (dominated by `git`); ~`356 KiB/op`, ~`2025 allocs/op` (`darwin/arm64`; requires `git`). Re-run with `-count=5` for allocator stability.
    - `BenchmarkScannerPreloadGitInfo_200Paths_5kExtraGitFiles`: ~`1.5–2.0 s/op` at `b.N=1`, ~`3.4 MiB/op`, ~`37k allocs/op` — stresses a large `git log` with only **200** layout paths; see `docs/perf/scanner-preload-gitinfo-plan.md`.
    - `BenchmarkCollectStatements_500Transactional`: ~`107 KiB/op`, ~`7 allocs/op` (500 `tables` objects, `BatchSize` 100, file content cached before `ResetTimer`; `darwin/arm64` spot check after raw checksum + lazy hex memo—re-run `make bench-perf-audit` for your machine)
    - `BenchmarkExecuteTxBatch_SuccessPath_100Statements`: ~`20.9 KiB/op`, ~`104 allocs/op` (pooled `strings.Builder` for `buildTxBatchSQL`); `ns/op` varies with load
    - `BenchmarkExecuteTxBatch_FailurePath_100Statements`: ~`43.7 KiB/op`, ~`220 allocs/op` (pooled builder for each single-statement `BEGIN`/`COMMIT` wrapper); `ns/op` dominated by 100 sequential `Exec` calls plus bus publishes
  - **Allocator / CPU profiles (2026-05-18)** — `docs/perf/runs/2026-05-18-apply-fs-profile/README.md` documents exact `go test -memprofile` / `-cpuprofile` commands and lists checked-in `*.prof` plus `go tool pprof -alloc_objects -top` text (`mem-allocobj-top-*.txt`). Highlights from that capture (historical): an older `collectStatements` path showed heavy `encoding/hex.EncodeToString` in `alloc_objects`; current `internal/apply/apply.go` avoids per-row hex during collect and memoizes hex once per `batchedStmt` on first bus event. `executeTxBatch` failure path still shows `newObjectEvent` and `buildSingleTxSQL` / `strings.(*Builder).Grow`; isolated `Layout.RebuildPathIndexes` shows `buildObjectsByPath` / `buildTransitionsByPath` (`internal/fs/layout_bench_test.go`).
  - **Bench vs saved audit baseline (2026-05-18)** — `docs/perf/runs/2026-05-18-baseline-vs-post/README.md` summarizes `benchstat` vs the excerpt from `docs/perf/runs/2026-05-18-audit-bench/bench-output.txt` (same `-count=5 -benchtime=150ms` harness). Apply package: `CollectStatements` **−10.76%** ns/op, **−50.05%** B/op, **−3.91%** allocs/op; `ExecuteTxBatch` success **−11.99%** B/op, **−6.31%** allocs/op; failure path **−6.10%** B/op, **−3.08%** allocs/op (`p=0.008`, `n=5`); wall-time on tx batch benches marked **~** (noise). Full tables: `benchstat-apply-only.txt`.
- Exit criteria:
  - Every performance claim in the audit is classified as either code-confirmed, unsupported, or benchmark-needed.
  - The benchmark backlog points to exact commands and exact repository paths.

## Operations And Recovery

- Routine operation:
  - Use this document when changing `internal/fs`, `internal/diff`, `internal/db`, `internal/apply`, `internal/bus`, or `internal/engine`.
  - Keep code-confirmed findings and benchmark-needed findings separate in PR descriptions and review notes.
  - When claiming a performance win, update `docs/profiling-benchmark-plan.md` or add a dated run record under `docs/perf/runs/`.
- Recovery or rollback:
  - If a proposed optimization is not backed by `go test -benchmem` or `pprof`, revert the claim to the benchmark-needed bucket instead of presenting it as a validated bottleneck.
  - If a new benchmark contradicts this audit, update this document and the dated evidence path in the same change.

## Open Issues And Non-Goals

- Open issues:
  - The current benchmark harness still mixes steady-state work with fixture setup for some profiles, as documented in `docs/profiling-benchmark-plan.md`.
- Non-goals:
  - This audit does not mandate a rewrite to buffered channels, ring buffers, tries, skip lists, or structure-of-arrays storage.
  - This audit does not claim end-to-end MSSQL migration throughput without live database profiling.

## References

- `internal/engine/engine.go`
- `internal/fs/layout.go`
- `internal/fs/scanner.go`
- `internal/fs/scanner_bench_test.go`
- `internal/fs/layout_bench_test.go`
- `internal/fs/normalize.go`
- `internal/diff/diff.go`
- `internal/apply/apply.go`
- `internal/apply/apply_bench_test.go`
- `internal/bus/bus.go`
- `internal/bus/bus_bench_test.go`
- `internal/db/inspector_impl.go`
- `internal/db/inspector_bench_test.go`
- `internal/types/chunk.go`
- `internal/engine/benchmark_profiling_test.go`
- `internal/types/chunk_bench_test.go`
- `docs/profiling-benchmark-plan.md`
- `Makefile`
- `docs/perf/runs/2026-05-18-audit-bench/bench-output.txt`
- `docs/perf/runs/2026-05-18-audit-bench/benchstat-before-add-apply-benches.txt`
- `docs/perf/runs/2026-05-18-audit-bench/benchstat-first-vs-latest.txt`
- `docs/perf/runs/2026-05-18-baseline-vs-post/README.md`
- `docs/perf/runs/2026-05-18-apply-fs-profile/README.md`
- External method reference: [Data Structures Optimization](https://psavelis.github.io/golang-performance-optimization/optimization/algorithms/data-structures.html)
