# Technical Document: Profiling And Benchmark Plan

Lifecycle: `Current`.

## Purpose

This document defines how to validate performance-sensitive paths in `rmig` with `make check`, Go benchmarks, and CPU or memory profiles. It records a fixed **baseline** from 2026-05-17 and a **current** measurement from the same methodology so regressions are comparable without chat history.

## Scope

- Correctness gate: `Makefile` target `check`
- Benchmarks and profiling harness: `internal/engine/benchmark_profiling_test.go`
- Optional repeatable bench/profile entrypoints: `Makefile` targets `bench-perf-diff-skip`, `bench-perf-diff-create`, `bench-perf-chunkkeys`, `bench-perf-audit` (internal bus, db, fs, apply backlog benches; see `docs/internal-performance-audit.md`), `profile-perf-diff-skip-cpu`, `profile-perf-diff-skip-mem` (see **Progress tracking (80/20)** below)
- Hot-path implementation under test: `internal/diff/diff.go`, `internal/fs/normalize.go`, `internal/fs/layout.go` (including `LayoutHash` digest buffer pool), `internal/types/chunk.go` (SQL parameter chunking for inspector fallback queries), `internal/db/inspector_impl.go` (cached `OPENJSON` capability probe plus JSON-parameter inspector reads)

## Progress tracking (80/20: CPU and heap evidence)

Use a short feedback loop so most effort goes where measurements show cost (CPU time or allocations), not where intuition guesses cache misses. The Go runtime does **not** expose hardware cache-miss counters; treat **CPU profile** (where cycles go) plus **struct and slice layout review** as the practical proxy for cache sensitivity, as in typical production Go workflows.

**Signals (priority order)**

1. **`go test -benchmem`** (or `make bench-perf-diff-skip` / `make bench-perf-diff-create`): **ns/op** stability, then **B/op** and **allocs/op** on the benchmark line. These are the primary regression metrics between commits.
2. **`benchstat`** on two benchmark logs (same flags, same machine class): statistical view of (1).
3. **CPU profile** (`profile-perf-diff-skip-cpu` or manual `-cpuprofile`): **flat** time in application packages (`diff`, `fs`, `db`) vs runtime (`gc`, `pthread_*`). Use **cum** to see callers.
4. **Heap profile** (`profile-perf-diff-skip-mem` or manual `-memprofile`): **`alloc_objects`** for steady-state allocators; **`alloc_space`** is often dominated by benchmark fixture setup—compare with benchmem **B/op** before over-interpreting `pprof` totals.
5. **Data-structure sanity**: only after (1)–(4), consider map vs slice, pointer slices vs value slices, or preallocation—see `internal/types/chunk.go` and inspector chunking.

**Minimal loop before merging hot-path changes**

1. `make check`
2. `make bench-perf-diff-skip` (planner steady path) and, if the change touches create/adopt/git, `make bench-perf-diff-create`
3. If numbers move: capture `profile-perf-diff-skip-cpu` and/or `profile-perf-diff-skip-mem`, open with `go tool pprof -http=:0`
4. Record `benchstat` old vs new when claiming a win (install: `go install golang.org/x/perf/cmd/benchstat@latest`)

**Chunking micro-benchmark:** `make bench-perf-chunkkeys` exercises `types.ChunkKeys` (inspector IN-list batching). Use it when changing chunking or parameter limits.

**Internal performance audit benches:** `make bench-perf-audit` runs the measured backlog set in `docs/internal-performance-audit.md` (packages `internal/bus`, `internal/db`, `internal/fs`, `internal/apply`; `BenchmarkScannerPreloadGitInfo_200Paths` needs `git` on `PATH`). Store dated stdout under `docs/perf/runs/` when you need a regression baseline outside chat.

**Apply + layout path indexes (`pprof`):** after allocator or `Layout` path-index changes, capture memory (and optionally CPU) profiles using the exact commands in `docs/perf/runs/2026-05-18-apply-fs-profile/README.md`. That run directory includes small binary `*.prof` files and checked-in `go tool pprof -text` summaries (`mem-allocobj-top-*.txt`) so reviewers see where allocations still land without re-running benches.

**Scanner batched `preloadGitInfo` (`pprof`):** for `BenchmarkScannerPreloadGitInfo_*`, use `docs/perf/runs/2026-05-18-scanner-preload-gitinfo-profile/README.md`. Prefer the **`*-steady`** captures (`-benchtime=3s` on the 200-path bench) when attributing **`alloc_objects`** to `preloadGitInfo` vs fixture; short `-benchtime` runs often have **`b.N==1`** and skew toward setup and tempdir cleanup.

**Bench delta vs saved audit stdout:** `docs/perf/runs/2026-05-18-baseline-vs-post/README.md` documents `benchstat` of the current tree against the excerpt from `docs/perf/runs/2026-05-18-audit-bench/bench-output.txt` (same `go test` flags). Use it when you need percentage deltas, not only `pprof` flat/cum tables.

**Inspector `OPENJSON` path:** `docs/perf/runs/2026-05-18-openjson-inspector/README.md` records the SQL-side inspector change (`internal/db/sql/*_openjson.sql`, cached compatibility probe, unit and integration coverage, and focused `BenchmarkInspectorInspect_*` reruns). Read it before interpreting inspector cold-path deltas from older audit captures.

Benchmarks build a temporary Git repo and scanned layout (`makeRealLayout` / `makeRealFS`), then call `diff.(*Computer).Compute` or `fs.NormalizeAndHash`. Profiles therefore include **fixture construction** (`makeBenchSQL`, `fmt.Sprintf`) as well as steady-state `Compute`; when reading `alloc_space`, always separate **one-time setup** from the **per-iteration** line in `go test -benchmem` output.

## SkipHeavy steady-state memory (three allocs and B/op)

This section ties **`BenchmarkDiffCompute_SkipHeavy_2000Objects`** (`internal/engine/benchmark_profiling_test.go`) to **`diff.(*Computer).Compute`** (`internal/diff/diff.go`) with **actionable** commands and arithmetic (not hand-waving).

### The three heap allocations per `Compute` call (skip-heavy, non-blocked)

 Preconditions for this benchmark: `state != nil`, `checksums != nil`, and `len(layout.Transitions)==0` (fixture has object kinds only; no `_migrations` SQL).

 Under those conditions, each `Compute` does **three** heap allocations:

| # | Code | What allocates |
|---|------|------------------|
| 1 | `plan := &types.MigrationPlan{ PlannedAt: … }` (`internal/diff/diff.go` lines 22–24) | The **`MigrationPlan` value** on the heap (returned pointer escapes). |
| 2 | `plan.Schemas = make([]types.PlannedSchema, 0, len(layout.Schemas))` (line 33) | Backing array for **schemas** (fixture has **one** schema row from `makeRealFS` → `testdb/schema/`). |
| 3 | `plan.Objects = make([]types.PlannedObject, 0, len(layout.Objects))` (line 69) | Backing array for **all planned objects** — this dominates **`B/op`**. |

 **`Blockers`:** left **nil** until the first blocked transition/trigger branch; `appendBlocker` (`internal/diff/diff.go` lines 190–195) allocates `make([]string, 0, 8)` only when a blocker message is appended. Skip-heavy fixtures never hit that path, so **`allocs/op` drops by one** vs eager `plan.Blockers = make(...)`.

 Not allocated here: `transitionsByKey` stays **nil** when `len(layout.Transitions)==0` (lines 45–53). The `state == nil` / `checksums == nil` branches (lines 26–31) are **not** taken because the benchmark builds maps before `ResetTimer()` (`internal/engine/benchmark_profiling_test.go` lines 62–83).

### Why `B/op` is about **427 KiB** for `n=2000`

Struct sizes ( **`go version go1.26.3 darwin/arm64`** — use `unsafe.Sizeof` locally on other GOOS/GOARCH):

| Type | `unsafe.Sizeof` |
|------|-----------------|
| `types.PlannedObject` | 240 bytes |
| `types.PlannedSchema` | 40 bytes |
| `types.MigrationPlan` (value layout) | 496 bytes |

`makeRealFS` with `n=2000` does **not** emit 2000 files: four kinds get `count = n/4` for `views` / `procedures` / `functions` (**500** each) and `count = n/4/2` for `tables` (**250**), with `objIdx < n`. Total objects: **500×3 + 250 = 1750** (`internal/engine/benchmark_profiling_test.go` `makeRealFS`).

Rough slice-only bytes: **`1750 × 240 B ≈ 420 KiB`** for `plan.Objects` backing storage, plus the **`MigrationPlan`** allocation, **`Schemas`** (one `PlannedSchema` value), and allocator alignment — this matches **`~427 KiB/op`** from `-benchmem` within a few percent (skip-heavy has **no** `Blockers` backing array).

### Exact commands (what to run, what to compare)

**A — Per-iteration truth (use this first)**

```bash
go test ./internal/engine -run '^$' -bench '^BenchmarkDiffCompute_SkipHeavy_2000Objects$' \
  -benchmem -count=5 -benchtime=400ms
```

Read **`B/op`** and **`allocs/op`** on the benchmark line only. That is **`Compute` with a fixed layout** after `b.ResetTimer()` in `benchmarkDiffComputeSkipHeavy`.

**B — `alloc_objects` profile (many small objects; fixture-heavy)**

```bash
go test ./internal/engine -run '^$' -bench '^BenchmarkDiffCompute_SkipHeavy_2000Objects$' \
  -count=1 -benchtime=200ms -memprofile=/tmp/mem-skip.prof
go tool pprof -top -flat -alloc_objects /tmp/mem-skip.prof
```

Expect **`makeBenchSQL`**, **`ReadDir`**, etc. to dominate **object count** — the profile window still includes **fixture** work around the benchmark, not only the timed loop. For **`Compute`’s allocation count**, trust **`allocs/op` from (A)**, not only the top of this list.

**C — `alloc_space` profile (do not equate flat `Compute` MiB with one call)**

```bash
go tool pprof -top -flat -alloc_space /tmp/mem-skip.prof
```

A large flat **`diff.(*Computer).Compute`** row is usually **the sum over many `Compute` invocations** in the profile window (each allocates a fresh **`[]PlannedObject`** backing of ~420 KiB × **`b.N`**) plus any overlapping samples, **not** “one `Compute` allocated 990 MiB”. Reconcile with (A):

`total_alloc_space_seen_for_Compute ≈ (B/op from A) × (how many times Compute ran during the profile) + setup/cleanup`

### If you change `types.PlannedObject`

1. Re-run **(A)**. For the same **1750**-object fixture, expect **`ΔB/op ≈ Δ(sizeof(PlannedObject)) × 1750`** (until padding changes).
2. **`allocs/op` should stay 3** on this skip-heavy path until you add new **per-call** heap allocs in `Compute` (for example storing an extra escaping pointer per object). Blocked runs add **at least one** alloc for the `Blockers` slice backing via `appendBlocker`.

## Benchmark semantics (read before profiling)

| Benchmark name | What it measures |
|----------------|------------------|
| `BenchmarkDiffCompute_*` (legacy names) | **Create-heavy:** empty `db.State.Objects` and empty `checksums` → every repo object is planned as `ActionCreateObject` (full `setGitInfo` / `PlannedObject` work per object). Still uses cached checksums after `Scanner.Scan` + `preloadChecksums`. |
| `BenchmarkDiffCompute_Create_*` | Same as above; name makes the scenario explicit. |
| `BenchmarkDiffCompute_SkipHeavy_*` | **Skip-heavy:** `state.Objects` and `checksums` are filled from the real layout so every object matches DB + prior checksum → `ActionSkipUnchanged` (no `setGitInfo` on the skip path). Use this to isolate planner overhead without the create branch. |
| `BenchmarkLayoutHash_*` | Repeated `Layout.LayoutHash()` only: **O(N log N)** sort over N digests + SHA-256 over sorted bytes each iteration. Heavy **GC cross-talk** if run in the same `go test` process immediately after `BenchmarkDiffCompute_*` because the diff benchmark leaves a large live heap. |
| `BenchmarkNormalizeAndHash_*` | In-memory normalize + hash of a single synthetic SQL string (size in name). |
| `BenchmarkChunkKeys_10k_2100` | `types.ChunkKeys` on 10k keys with chunk size 2100 (typical MSSQL parameter batching); isolates outer-slice growth for inspector queries. |

**GC and run order:** `go test` runs benchmarks in declaration order (or name order when using a regex). A long, alloc-heavy benchmark perturbs later ones. For trustworthy numbers, run **one** `-bench '^ExactName$'` per process, or run the slowest benchmark last.

**Profiling:** capture CPU and memory profiles in **separate** invocations per benchmark (see **Per-benchmark commands** below). `-cpuprofile` / `-memprofile` increase wall time; use them to find hot nodes, not to compare absolute ns/op against a non-profiled run.

## Per-benchmark commands

From the repository root (replace `benchtime` / `count` as needed):

```bash
# Diff — create-heavy (legacy DiffCompute_2000 name)
go test ./internal/engine -run '^$' -bench '^BenchmarkDiffCompute_2000Objects$' -benchmem -benchtime=400ms -count=5

# Diff — create-heavy (explicit)
go test ./internal/engine -run '^$' -bench '^BenchmarkDiffCompute_Create_2000Objects$' -benchmem -benchtime=400ms -count=5

# Diff — skip-heavy
go test ./internal/engine -run '^$' -bench '^BenchmarkDiffCompute_SkipHeavy_2000Objects$' -benchmem -benchtime=400ms -count=5

# LayoutHash only (avoid running after DiffCompute in the same command)
go test ./internal/engine -run '^$' -bench '^BenchmarkLayoutHash_2000Objects$' -benchmem -benchtime=400ms -count=5

# Normalize only
go test ./internal/engine -run '^$' -bench '^BenchmarkNormalizeAndHash_MediumSQL$' -benchmem -benchtime=400ms -count=5
```

**CPU + memory profile (one benchmark per invocation):**

```bash
go test ./internal/engine -run '^$' -bench '^BenchmarkDiffCompute_2000Objects$' \
  -benchtime=50ms -count=1 -cpuprofile=/tmp/cpu-diff.prof -memprofile=/tmp/mem-diff.prof
go tool pprof -http=:0 /tmp/cpu-diff.prof
go tool pprof -http=:0 -alloc_objects /tmp/mem-diff.prof
```

Repeat with `-bench '^BenchmarkLayoutHash_2000Objects$'` and separate output files.

**Reading `pprof` for these paths:**

- **CPU (flat vs cum):** look for `diff.(*Computer).Compute` vs runtime (`pthread_cond_*`, `madvise`, `gc`). Under `Compute`, expect `setGitInfo` / git accessors on create-heavy; on skip-heavy, `isMatch` and `decodeHex32` dominate among app code.
- **Memory `alloc_objects`:** separate `engine.makeBenchSQL` / fixture setup from `diff.Compute` or `LayoutHash`. String-based `LayoutHash` (historical) showed massive `hex.EncodeToString` and `sort.Strings`; current `LayoutHash` uses fixed `[32]byte` sorting.
- **`alloc_space`:** often dominated by one-time test setup; prefer **benchmem B/op and allocs/op** on the benchmark line for steady-state.

## Comparing two runs with benchstat

Install: `go install golang.org/x/perf/cmd/benchstat@latest`

Capture two logs (same machine, same flags), e.g. before/after a change or two commits checked out in worktrees:

```bash
go test ./internal/engine -run '^$' -bench '^BenchmarkLayoutHash_2000Objects$' \
  -benchmem -count=10 -benchtime=300ms 2>&1 | tee /tmp/bench-old.txt
# … switch revision …
go test ./internal/engine -run '^$' -bench '^BenchmarkLayoutHash_2000Objects$' \
  -benchmem -count=10 -benchtime=300ms 2>&1 | tee /tmp/bench-new.txt
$(go env GOPATH)/bin/benchstat /tmp/bench-old.txt /tmp/bench-new.txt
```

`benchstat` expects the standard `go test` benchmark lines; do not mix unrelated benchmarks in the same file if you only care about one function.

## Interfaces And Boundaries

- Inputs: local Go toolchain (`go test`, `go tool pprof`), optional `benchstat` from `golang.org/x/perf/cmd/benchstat`
- Outputs: stdout benchmark lines, optional `*.prof` files for `pprof`
- Ownership boundaries: benchmark numbers are **machine-dependent**; this repository stores methodology and baseline tables, not CI-enforced thresholds

## Assumptions And Constraints

- Assumptions: same major hardware class (e.g. Apple Silicon arm64) when comparing to the captured baseline; otherwise compare **ratios** (allocs/op, B/op) cautiously
- Constraints: `-cpuprofile` / `-memprofile` **distort** wall time; use plain `go test -bench` for ns/op regression detection, and profiles for **where** time or allocations go
- The 2026-05-17 baseline analysis attributed high `pthread_cond_*` and `diff.Compute` `alloc_objects` to **per-call goroutine fan-out** in `Compute`. Current code uses `warmupIfNeeded` so that, after `Scanner` preload, **no goroutines** are spawned for the hot path (see `internal/diff/diff.go`). When warmup does run, **`warmupAll`** now uses a **bounded worker pool** over a channel instead of **one goroutine per object**.

## Nominal Flow

1. Run `make check` from the repository root
2. Run benchmarks with memory metrics (repeat `-count` for stability). Prefer **one benchmark per command** (see **Per-benchmark commands** above) so GC and run order do not skew `LayoutHash` after `DiffCompute`. A combined smoke run is acceptable for a quick sanity check only:

   ```bash
   go test ./internal/engine -run '^$' \
     -bench 'BenchmarkDiffCompute_2000Objects|BenchmarkNormalizeAndHash_SmallSQL|BenchmarkNormalizeAndHash_MediumSQL|BenchmarkNormalizeAndHash_LargeSQL' \
     -benchmem -count=5 -benchtime=400ms
   ```

3. Capture profiles (expect higher ns/op than step 2 because profiling adds overhead):

   ```bash
   rm -f /tmp/rmig_cpu.prof /tmp/rmig_mem.prof
   go test ./internal/engine -run '^$' -bench BenchmarkDiffCompute_2000Objects \
     -benchtime=3s -count=1 \
     -cpuprofile=/tmp/rmig_cpu.prof -memprofile=/tmp/rmig_mem.prof
   go tool pprof -top -flat -nodecount=25 /tmp/rmig_cpu.prof
   go tool pprof -top -flat -alloc_space -nodecount=25 /tmp/rmig_mem.prof
   go tool pprof -top -flat -alloc_objects -nodecount=25 /tmp/rmig_mem.prof
   ```

## Off-Nominal Behavior And Failure Containment

- Failure mode: `make check` fails (test, vet, `staticcheck`, or fmt drift)
  Containment: fix before treating benchmark results as release-ready; do not retune hot paths on a red tree
- Failure mode: benchmark variance spikes (thermal throttling, background CPU)
  Containment: increase `-benchtime`, close other apps, compare multiple `-count` runs or use `benchstat`
- **Plan JSON (reports):** `types.PlannedObject` no longer has a separate `SourceFile` field; the script path is only `ObjectRef.ObjectPath` (JSON key `ObjectPath`). External tools that parsed `SourceFile` from `.plan.json` must switch to `ObjectPath`.
- **Go API (in-memory plan):** git metadata is `PlannedObject.Git *GitInfo` (nil when unset). JSON in `.plan.json` still uses the historical **flat** keys `GitHash`, `GitAuthor`, `GitDate` via `PlannedObject.MarshalJSON` / `UnmarshalJSON` in `internal/types/planned_json.go`. Call sites use `PlannedObject.GitStrings()` where raw strings are needed (for example `internal/apply`).

## Verification And Validation

- Contracts and checks: `make check` (see `Makefile`)
- Evidence artifacts: benchmark stdout, optional `*.prof` and `pprof` text reports
- Exit criteria: `make check` passes; benchmark table appended or updated under **Results** for the run date

## Operations And Recovery

- Routine operation: run nominal flow before merging changes that touch `internal/diff`, `internal/fs`, or benchmark harnesses
- Recovery: if profiles are misleading, re-run step 2 **without** `-cpuprofile` for authoritative ns/op and B/op

## Open Issues And Non-Goals

- Open issues: `alloc_space` profiles still dominated by `engine.makeBenchSQL` (fixture); use **benchmem B/op and allocs/op** for `Compute` steady state and `alloc_objects` to see `diff.Compute` share
- Open issues: `layoutHashDigestsPool` may retain a large backing array after hashing a very large layout until that buffer is reused or GC collects an abandoned oversized allocation (when `need` exceeds the pooled slice capacity and `make` allocates a new slice)
- Evidence (2026-05-17, profiling-driven): removed duplicate `SourceFile` from `types.PlannedObject` (apply uses `ObjectRef.ObjectPath`); `BenchmarkDiffCompute_SkipHeavy_2000Objects` steady-state **B/op** dropped by **32 KiB/op** (one `string` header per object) on `darwin/arm64` while **allocs/op** was **4** at that milestone (later **3** with lazy `Blockers`; see **SkipHeavy steady-state memory**); `diff.(*Computer).Compute` skips building `transitionsByKey` when `len(layout.Transitions)==0`
- Evidence (2026-05-17, optional `Git *GitInfo`): same benchmark **B/op** further reduced to **~426 KiB/op** (from **~492 KiB/op** after the `SourceFile` removal); lazy `Blockers` via `appendBlocker` reduced skip-heavy steady-state **`allocs/op` from 4 to 3**; **create-heavy** paths pay **~1 extra heap alloc per object** with git metadata (`Git` pointer + `GitInfo` value) — see `BenchmarkDiffCompute_Create_2000Objects` `allocs/op` for regression tracking
- Non-goals: this document does not set automatic performance gates in CI

## References

- `Makefile` (`check` target; performance targets `bench-perf-*`, `profile-perf-*`)
- `internal/engine/benchmark_profiling_test.go`
- `internal/types/planned_json.go` (`PlannedObject` JSON compatibility, `GitStrings`)
- `internal/types/chunk_bench_test.go` (`BenchmarkChunkKeys_10k_2100`)
- `internal/diff/diff.go` (`warmupIfNeeded`, `isMatch`, `decodeHex32`, transition index build)
- `internal/apply/apply.go` (layout lookup by `PlannedObject.ObjectRef.ObjectPath`; skips building `ObjectEvent` / `FailureEvent` when `bus.EventBus.HasHandlers` is false for that event; zero digest checksum uses constant hex instead of `hex.EncodeToString`)
- `internal/bus/bus.go` (`EventBus.HasHandlers`, `Publish` no-op when no subscribers)
- Prior narrative analysis (goroutine fan-out, `hex.EncodeToString`, `LayoutHash`): retained as **historical context** in section **Baseline analysis (2026-05-17)**
- Full profiling round (benchmark matrix, CPU/mem text extracts, triage A–E): `docs/perf/runs/2026-05-17-profiling-round/TRIAGE.md`
- External methodology (measure → profile → optimize → validate): [Optimization Strategies](https://psavelis.github.io/golang-performance-optimization/optimization/), [algorithms](https://psavelis.github.io/golang-performance-optimization/optimization/algorithms/), [data structures](https://psavelis.github.io/golang-performance-optimization/optimization/algorithms/data-structures.html), [space complexity](https://psavelis.github.io/golang-performance-optimization/optimization/algorithms/space-complexity.html)

## Profiling run record (matrix: 2026-05-17)

This run executed the roadmap: baseline `benchmem` per benchmark (separate processes), `-cpuprofile` / `-memprofile` per benchmark, `pprof` text summaries, `benchstat` where applicable, triage, and code changes.

**Environment:** `go1.26.3 darwin/arm64`, Apple M2, `GOMAXPROCS=8`.

**Artifact directory:** `docs/perf/runs/2026-05-17-profiling-round/`

| Artifact type | Files | Notes |
|---------------|-------|--------|
| Baseline stdout (pre-fix) | `baseline-Benchmark*.txt` | `-count=10`, `-benchtime=200ms`, `-benchmem` |
| Baseline stdout (post-fix) | `baseline-POSTFIX-Benchmark*.txt` | Same flags after patches |
| CPU | `cpu-*.prof` (binary), `cpu-topcum-*.txt`, `cpu-run-*.txt` | `*.prof` is **gitignored** (see repository `.gitignore`); regenerate with per-bench `-cpuprofile` if you need interactive flame (`go tool pprof -http=:0`) |
| Memory | `mem-*.prof` (binary), `mem-allocobj-top-*.txt`, `mem-allocspace-top-*.txt`, `mem-run-*.txt` | `alloc_objects` separates fixture (`makeBenchSQL`) from steady-state `Compute` / `LayoutHash` |
| Comparison | `benchstat-Create.txt`, `benchstat-SkipHeavy.txt` | `benchstat` from `golang.org/x/perf/cmd/benchstat` |
| Triage | `TRIAGE.md` | Hypotheses A–E with accept/reject and evidence pointers |

**Commands to reproduce profiles (example):**

```bash
OUT=docs/perf/runs/2026-05-17-profiling-round
B=BenchmarkDiffCompute_SkipHeavy_2000Objects
go test ./internal/engine -run '^$' -bench "^${B}$" -count=1 -benchtime=150ms -cpuprofile="$OUT/cpu-${B}.prof"
go tool pprof -top -cum -nodecount=35 "$OUT/cpu-${B}.prof" > "$OUT/cpu-topcum-${B}.txt"
go test ./internal/engine -run '^$' -bench "^${B}$" -count=1 -benchtime=150ms -memprofile="$OUT/mem-${B}.prof"
go tool pprof -top -flat -alloc_objects -nodecount=30 "$OUT/mem-${B}.prof" > "$OUT/mem-allocobj-top-${B}.txt"
```

**Code changes tied to this run**

- `internal/diff/diff.go`: skip-unchanged path avoids a second `decodeHex32` on the same 64-char prior checksum; reuses one `Checksum` result (`benchstat` on SkipHeavy: ~−10% ns/op vs pre-fix baseline, `n=10`).
- `internal/engine/benchmark_profiling_test.go`: `makeBenchSQL` uses `strings.Builder` instead of per-line `string +=` (removes O(n²) string growth when generating each SQL fixture file).

**benchstat caveat:** comparing `baseline-BenchmarkDiffCompute_Create_2000Objects.txt` to POSTFIX in a later long session showed a large ns/op delta driven by **machine variance**; B/op and allocs/op were unchanged. Use back-to-back runs or controlled hardware for strict Create regressions.

---

## Baseline table (instrumented run, 2026-05-17)

Source: internal analysis (Apple M2, arm64, 8 P-cores). Benchmarks as named in `benchmark_profiling_test.go`.

| Benchmark | Time/op | B/op | Allocs/op |
|-----------|---------|------|------------|
| `BenchmarkDiffCompute_2000Objects` | 1 232 892 ns | 606 811 | 1 757 |
| `BenchmarkNormalizeAndHash_SmallSQL` (~200 B SQL) | 43 353 ns | 0 | 0 |
| `BenchmarkNormalizeAndHash_MediumSQL` (~2 KiB SQL) | 441 584 ns | 36 | 0 |
| `BenchmarkNormalizeAndHash_LargeSQL` (~20 KiB SQL) | 4 479 930 ns | 4 895 | 0 |

**Baseline profiling interpretation (same date):**

- CPU: large `pthread_cond_wait` / `pthread_cond_signal` share tied to **2000 goroutines per `Compute`** on the cold fan-out path
- Memory: `diff.(*Computer).Compute` ~3.1% `alloc_space` vs `makeBenchSQL` ~95.9%; `alloc_objects` ~49.4% from `go func` in `Compute`
- Recommended changes included: conditional warmup instead of unconditional goroutine fan-out; avoid `hex.EncodeToString` in checksum compare; reduce `LayoutHash` intermediate string allocations

---

## Current table (repository state, 2026-05-17)

Same commands as nominal flow step 2 (`-count=5`, `-benchtime=400ms`), machine: **Apple M2**, `goos: darwin`, `goarch: arm64`.

| Benchmark | Time/op (range over 5 runs) | B/op | Allocs/op |
|-----------|-----------------------------|------|------------|
| `BenchmarkDiffCompute_2000Objects` | 108 580–109 647 ns (~**109** µs) | 525 040 | **3** |
| `BenchmarkNormalizeAndHash_SmallSQL` | 42 969–43 031 ns | 0–3 | 0 |
| `BenchmarkNormalizeAndHash_MediumSQL` | 430 919–431 725 ns | 456–460 | 0 |
| `BenchmarkNormalizeAndHash_LargeSQL` | 4 379 289–4 391 542 ns | 60 883–71 358 | 0 |

**Delta vs baseline (same benchmarks):**

| Metric | Baseline | Current | Notes |
|--------|----------|---------|--------|
| `DiffCompute_2000` time/op | ~1.23 ms | ~**0.109** ms | ~**11×** faster (warm path avoids goroutine storm) |
| `DiffCompute_2000` B/op | ~607 KiB | ~513 KiB | ~**14%** lower |
| `DiffCompute_2000` allocs/op | 1 757 | **3** | aligns with removal of per-object goroutine churn on cached layout |
| Normalize benchmarks | similar order of magnitude | similar | dominated by `normalizeSQLBytes` + SHA-256; small drift in reported B/op is normal |

**Current profile snapshot** (`BenchmarkDiffCompute_2000Objects`, `-benchtime=3s`, `-cpuprofile` / `-memprofile`; CPU top flat): `runtime.madvise`, `pthread_cond_*`, and `syscall.rawsyscalln` still appear (runtime, GC, OS, profiling), but `internal/diff.(*Computer).Compute` **cum** was **~9%** in this capture vs being the structural center of the old flame graph. **`alloc_objects`**: `diff.(*Computer).Compute` **~0.77%** flat vs **~49%** from the old `go func` line in the baseline write-up fixture profile.

---

## Historical note

The baseline document’s flame graph image paths pointed outside this repository; this plan intentionally stores **tables and commands** only so evidence remains reproducible from the repo.
