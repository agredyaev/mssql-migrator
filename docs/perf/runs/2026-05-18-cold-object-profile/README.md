# Cold object path + layout rebuild: CPU / memory / padding (2026-05-18)

## What this is

A focused profiling pass on `internal/fs` after the object checksum path retains file bytes for `Content()` (`(*Object).Checksum)` + shared raw-byte cache). It answers where CPU, `alloc_space`, `alloc_objects`, and struct padding noise come from on this capture machine.

## Why it exists

Micro-benchmarks showed a large wall-time gap between `Checksum`→`Content` on a bare `CachedFile` (two reads) vs `*Object` (one read). Profiles explain what dominates **after** that change and what still looks expensive or allocation-heavy.

## How it was produced

Machine note: capture below used `darwin/arm64` (Apple M2). Absolute `ns/op` will differ on other CPUs; relative stacks are still useful.

```bash
DOC=docs/perf/runs/2026-05-18-cold-object-profile
mkdir -p "$DOC"

# Cold path: two file reads (baseline sub-benchmark)
go test ./internal/fs -run '^$' \
  -bench '^BenchmarkCachedFileChecksumThenContent_ColdObject/current$' \
  -cpuprofile="$DOC/cpu-current.prof" -memprofile="$DOC/mem-current.prof" \
  -benchmem -benchtime=3s -count=1

# Cold path: object retains bytes (retain sub-benchmark)
go test ./internal/fs -run '^$' \
  -bench '^BenchmarkCachedFileChecksumThenContent_ColdObject/retainBytes$' \
  -cpuprofile="$DOC/cpu-retain.prof" -memprofile="$DOC/mem-retain.prof" \
  -benchmem -benchtime=3s -count=1

# Path index rebuild (500 objects)
go test ./internal/fs -run '^$' -bench '^BenchmarkLayoutRebuildPathIndexes_500Objects$' \
  -cpuprofile="$DOC/cpu-layout-rebuild.prof" -memprofile="$DOC/mem-layout-rebuild.prof" \
  -benchmem -benchtime=2s -count=1

# Struct field order / padding hints (install once: same module path as upstream docs)
go install golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@latest
"$(go env GOPATH)/bin/fieldalignment" ./internal/fs/...
```

Summaries used in this note:

```bash
go tool pprof -top -nodecount=30 "$DOC/cpu-current.prof"
go tool pprof -top -alloc_objects -nodecount=30 "$DOC/mem-current.prof"
go tool pprof -top -alloc_space -nodecount=25 "$DOC/mem-current.prof"

go tool pprof -top -nodecount=30 "$DOC/cpu-retain.prof"
go tool pprof -top -alloc_objects -nodecount=30 "$DOC/mem-retain.prof"
go tool pprof -top -alloc_space -nodecount=20 "$DOC/mem-retain.prof"

go tool pprof -top -nodecount=25 "$DOC/cpu-layout-rebuild.prof"
go tool pprof -top -alloc_objects -nodecount=25 "$DOC/mem-layout-rebuild.prof"
```

## Findings

### A. `ColdObject/current` (two `ReadFile` passes)

- **CPU (flat):** almost everything under `syscall.rawsyscalln` → `os.ReadFile` → `(*CachedFile).Checksum` / `Content`. Two full open/read/stat/close cycles per benchmark iteration.
- **`alloc_objects`:** dominated by `os.ReadFile` / `os.Open` / `os.newFile` / `os.(*File).Stat` / `os.readFileContents` — again consistent with **two** reads per iteration.
- **`alloc_space`:** same story — duplicate file buffers and syscall scaffolding.

**Interpretation:** “noise” here is mostly **real I/O cost**, not Go hash maps. Optimizations are: avoid second read (object retain path), `mmap` (rarely worth it for small SQL), or reuse `[]byte` at a higher layer (layout cache already does for repeated scans).

### B. `ColdObject/retainBytes` (one read + cache hit on every new `*Object`)

- **CPU (flat):** ~95% `syscall.rawsyscalln` under `os.Stat` from `lookupSharedObjectBytes` (`internal/fs/scanner.go`), not under `ReadFile`, once the shared cache is warm.
- **`alloc_objects` / `alloc_space`:** `syscall.ByteSliceFromString` + `os.statNolog` dominate — one **`os.Stat(path)` per `Checksum` call** on the cache-hit path to compare stored `size`/`mtime` with the file.

**Interpretation:** the micro-bench constructs a **new** `*Object` each iteration **without** running `Scanner.Scan`, so **`objectStatForByteCacheValid` is false** and the cache-hit path still calls `os.Stat` every time. After a real `Scan`, hints allow skipping `Stat` on hit (see **Validation** below).

**Candidate optimizations (trade-offs):**

1. ~~**Cheaper invalidation**~~ — superseded by stat hints from layout `fileStates` when `Scan` / cached layout load attaches hints.
2. ~~**Avoid per-hit `Stat` when layout file-state already proved freshness**~~ — implemented via `attachObjectByteCacheStatHints` + `lookupSharedObjectBytes(path, hint)`.
3. **Reduce string→syscall conversions** — still relevant when the hint path is off; less important on the hint fast path.

### C. `LayoutRebuildPathIndexes` (500 objects)

- **CPU top:** heavy on runtime (`kevent`, `pthread_cond_*`, `madvise`, `mapassign_faststr`) — typical **GC / scheduler / map growth** noise on a tight loop, not a single obvious application hot spot.
- **`alloc_objects`:** `buildObjectsByPath` and `buildTransitionsByPath` dominate; `rebuildContentRetainPathLists` adds **~15%** of allocation objects in this capture (sorted `AbsPath` slices duplicated from existing `Object` / `Transition` / `Check` structs).

**Candidate optimizations:**

1. **Lazy or incremental retain lists** — only rebuild sorted slices when layout mutation APIs are used; `Scan` already knows object paths in order sometimes (trade complexity).
2. **Store indices instead of string slices** — implemented as `retainObjectOrder` / `nonObjectOrder` (`internal/fs/layout.go`); `rebuildContentRetainPathLists` still allocates for `sort.Slice` reflect swapper (see post-capture `mem-allocobj-top-post-LayoutRebuildPathIndexes.txt`).

### D. Padding / struct layout (`fieldalignment`)

Representative output on this tree:

| Location | Message |
| --- | --- |
| `internal/fs/layout.go` (`CachedFile`) | struct with **240** pointer bytes could be **144** |
| `internal/fs/scanner.go` (`Scanner`, caches, entries) | several structs “could be” smaller by reordering fields |

**Interpretation:** reordering fields can reduce **struct size and padding** and slightly improve cache footprint. Wins are usually **smaller than I/O or map rebuild** costs unless hot structs are allocated at very high QPS. Apply `fieldalignment -fix` only with review — it changes field order and can break `unsafe` or binary assumptions (none known here, but verify).

## What this does not cover

- End-to-end `plan` / `apply` / MSSQL latency (different dominant costs).
- Linux vs Windows syscall shapes (paths differ).
- `benchstat` significance — these profiles are **single** `-count=1` runs for file size; repeat with `-count=5` + `benchstat` when claiming regressions.

## References

- Benchmarks: `internal/fs/layout_bench_test.go` (`BenchmarkCachedFileChecksumThenContent_ColdObject`, `BenchmarkLayoutRebuildPathIndexes_500Objects`, `BenchmarkObjectChecksumSharedBytesAfterScan`).
- Cache hit path: `lookupSharedObjectBytes` / `storeSharedObjectBytesWithStat` in `internal/fs/scanner.go`.
- **Binary `*.prof` files are gitignored** (`*.prof` in repo `.gitignore`); this directory stores **text extracts** (`cpu-top-*.txt`, `mem-allocobj-top-*.txt`). Regenerate profiles under `/tmp` and re-run `go tool pprof` as in **Validation** below.

## Validation (post stat-hint + index lists, same machine class)

Re-run on your machine; numbers below are one capture (`darwin/arm64`, Apple M2).

### A. Build a symbolized test binary (required for readable `pprof` stacks)

```bash
go test -c -o /tmp/fs-bench.test ./internal/fs
```

### B. Stat-hint fast path vs no hint (same warm shared byte cache)

Benchmark: `BenchmarkObjectChecksumSharedBytesAfterScan` in `internal/fs/layout_bench_test.go` (`withScanHint` = hints copied from a real `Scan`; `withoutScanHint` = same but `objectStatForByteCacheValid` cleared).

```bash
go test ./internal/fs -run '^$' \
  -bench '^BenchmarkObjectChecksumSharedBytesAfterScan/' \
  -benchmem -benchtime=500ms -count=3
```

Example stdout is also checked in as `bench-post-validation-ScanHint.txt` (same numbers as below).

CPU extracts (symbolized):

```bash
go test ./internal/fs -run '^$' \
  -bench '^BenchmarkObjectChecksumSharedBytesAfterScan/withScanHint$' \
  -cpuprofile=/tmp/cpu-scanhint.prof -memprofile=/tmp/mem-scanhint.prof \
  -benchmem -benchtime=3s -count=1

go test ./internal/fs -run '^$' \
  -bench '^BenchmarkObjectChecksumSharedBytesAfterScan/withoutScanHint$' \
  -cpuprofile=/tmp/cpu-no-hint.prof -benchmem -benchtime=2s -count=1

go tool pprof -top -nodecount=35 /tmp/fs-bench.test /tmp/cpu-scanhint.prof \
  | tee docs/perf/runs/2026-05-18-cold-object-profile/cpu-top-post-ScanWithHint.txt
go tool pprof -top -nodecount=20 /tmp/fs-bench.test /tmp/cpu-no-hint.prof \
  | tee docs/perf/runs/2026-05-18-cold-object-profile/cpu-top-post-ScanWithoutHint.txt
```

**Read-off:** `cpu-top-post-ScanWithHint.txt` — top flat time is **runtime / hashing**, not `os.Stat` under `lookupSharedObjectBytes` (that frame is ~**0.05 s cum** in this capture vs **~2.14 s cum** for `os.Stat` in `cpu-top-post-ScanWithoutHint.txt`).

`mem-allocobj-top-post-ScanWithHint.txt` — dominated by allocating the fresh `*Object` wrapper each iteration (`benchCopyObjectForChecksumLoop`), not syscall string conversions from `Stat`.

### C. `LayoutRebuildPathIndexes` allocator snapshot (index-based retain lists)

```bash
go test ./internal/fs -run '^$' \
  -bench '^BenchmarkLayoutRebuildPathIndexes_500Objects$' \
  -memprofile=/tmp/mem-layout-rebuild-post.prof -benchmem -benchtime=3s -count=1

go tool pprof -top -alloc_objects -nodecount=40 /tmp/fs-bench.test /tmp/mem-layout-rebuild-post.prof \
  | tee docs/perf/runs/2026-05-18-cold-object-profile/mem-allocobj-top-post-LayoutRebuildPathIndexes.txt
```

**Read-off:** `rebuildContentRetainPathLists` still shows up under `alloc_objects` mostly from **`sort.Slice`** (`internal/reflectlite.Swapper`) over index permutations — no duplicate `AbsPath` **string** slice growth; dominant allocs remain **`buildObjectsByPath` / `buildTransitionsByPath`** map rebuilds.

### D. `benchstat` — два подряд прогона одного и того же бенча (`n=10`)

`benchstat` сравнивает **одинаковые имена** бенчей в двух файлах (два прогона на одной машине). Здесь это проверка **шума**, а не A/B двух разных веток кода.

```bash
DOC=docs/perf/runs/2026-05-18-cold-object-profile
B='^BenchmarkObjectChecksumSharedBytesAfterScan$'
for i in 1 2; do
  go test ./internal/fs -run '^$' -bench "$B" -benchmem -count=10 -benchtime=200ms 2>&1 \
    | tee "$DOC/bench-scanhint-run${i}.txt"
done
go install golang.org/x/perf/cmd/benchstat@latest
"$(go env GOPATH)/bin/benchstat" "$DOC/bench-scanhint-run1.txt" "$DOC/bench-scanhint-run2.txt" \
  | tee "$DOC/benchstat-scanhint-run1-vs-run2.txt"
```

**Чтение последнего снимка:** `withScanHint` — `ns/op` **`~`** между сессиями; `withoutScanHint` — небольшая дельта порядка **1%** при `p<0.05` (машинный шум). `B/op` и `allocs/op` совпали между прогонами.

Сравнить **`withScanHint`** vs **`withoutScanHint`** одной парой файлов `benchstat` **нельзя** (разные имена подбенчей); для отношения скорости используй один stdout с обоими подбенчами и **`bench-post-validation-ScanHint.txt`**, либо деление медиан вручную.

## How to validate

Use the **Validation** commands above after a change; compare `cpu-top-post-ScanWithHint.txt` vs `cpu-top-post-ScanWithoutHint.txt` for `os.Stat`, and `mem-allocobj-top-post-LayoutRebuildPathIndexes.txt` for `rebuildContentRetainPathLists` vs `buildObjectsByPath`. For run-to-run stability on the Scan-hint benchmark pair, regenerate `bench-scanhint-run*.txt` and `benchstat-scanhint-run1-vs-run2.txt`.

## Follow-up implemented in code (same repo)

1. **`lookupSharedObjectBytes`:** optional stat hint from `Object.objectStatForByteCache` (filled by `attachObjectByteCacheStatHints` from `buildLayoutFileCacheState` after `preloadChecksums`, and when loading a cached layout in `tryLayoutCacheEntry`). When hint matches the cache entry’s stored `size`/`modTime`, the hot path skips `os.Stat` (caller contract: hint comes from the same Scanner snapshot as layout validation).
2. **Single open/read for object checksum cold path:** `readFileBytesAndStat` + `storeSharedObjectBytesWithStat` avoid an extra `os.Stat` after `os.ReadFile` for retained object bytes.
3. **`RebuildPathIndexes` retain lists:** `Layout.retainObjectOrder` / `Layout.nonObjectOrder` store sorted **indices** into `Objects` / `Transitions` / `Checks` instead of duplicating every `AbsPath` string.

Tests: `TestLookupSharedObjectBytesHintMatchesCacheEntry`, `TestRebuildPathIndexesRetainOrderSortedByAbsPath`, `TestScanAttachesObjectByteCacheStatHints` in `internal/fs/fs_test.go`.

