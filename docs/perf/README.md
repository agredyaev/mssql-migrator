# Performance evidence index (`docs/perf/`)

## What this is

A single entry point for **benchmark deltas**, **`pprof` captures**, and links into the performance audit. If you expected a large wall-clock speedup on real migrations and do not see it, read **“What did not get faster”** below first.

## Where the numbers are

| Question | Document / artifact |
| --- | --- |
| **Before vs after (apply benches)** | `runs/2026-05-18-baseline-vs-post/README.md` — start with the **TL;DR** table (memory and allocs are the stable wins). Raw `benchstat`: `runs/2026-05-18-baseline-vs-post/benchstat-apply-only.txt`. |
| **Where allocations still go (`pprof`)** | `runs/2026-05-18-apply-fs-profile/README.md` and `mem-allocobj-top-*.txt` in that folder. |
| **`preloadGitInfo` (`git log` batch path)** | `runs/2026-05-18-scanner-preload-gitinfo-profile/README.md` — steady-state **`alloc_objects`** / CPU for `BenchmarkScannerPreloadGitInfo_200Paths`, plus short-window and **5k-noise** captures (read the README before interpreting `b.N==1` profiles). |
| **Full audit bench log (historical)** | `runs/2026-05-18-audit-bench/bench-output.txt`. |
| **Narrative audit** | `../internal-performance-audit.md` (repo root `docs/`). |
| **Planned next steps for `scopeKey` (inspector cache key)** | `scopekey-optimization-plan.md` — phased goals, cost drivers, verification commands. |
| **Planned next steps for batched `preloadGitInfo` (scanner)** | `scanner-preload-gitinfo-plan.md` — `git log` parse path, wanted-path filter, benchmark harness. |

## What actually improved (micro-benches, same machine, `n=5`)

From `benchstat` on `internal/apply` (`runs/2026-05-18-baseline-vs-post/benchstat-apply-only.txt`), relative to the saved audit excerpt:

- **`BenchmarkCollectStatements_500Transactional`:** **−50% B/op**, **−4% allocs/op**, **−11% ns/op** (all `p=0.008` in that capture).
- **`BenchmarkExecuteTxBatch_*`:** **−6% to −12% B/op**, **−3% to −6% allocs/op**; **ns/op marked “~”** (not statistically proven faster/slower at `n=5`).

Those are **allocator / footprint** wins on synthetic harnesses, not a promise that `rmig` finishes twice as fast on a real SQL Server.

## What did not get faster (expectations)

- **End-to-end migration time** is dominated by database I/O, network, and script size — not by the apply allocator or the in-memory path maps.
- **`ExecuteTxBatch` wall time** in the benchmark is noisy and was **not** a clear win in `benchstat` (`~` rows); look at **B/op** and **allocs/op** there instead.
- **Path index caching on `Layout`** removes **two `map` builds per `apply.Execute`** only when the layout came from **`Scanner.Scan`** (indexes built at end of scan). It does **not** change `BenchmarkCollectStatements_500Transactional`, which builds its own layout and never calls `Execute`.

## How to validate on your machine

```bash
# Same harness as the saved audit + delta docs
make bench-perf-audit

# Regenerate baseline-vs-post (see runs/2026-05-18-baseline-vs-post/README.md)
```

## Access-pattern and pre-size follow-ups (2026-05-18)

- **`internal/db/inspector_impl.go` (`readState`)** — `allObjNames` / `allTblNames` use `make(..., 0, sum(len(per-schema slice)))` before flattening schema chunks, avoiding repeated growth while concatenating name lists.
- **Same file (`querySchemas` / `queryObjects` / `queryColumns`)** — result maps use **hint capacity** from the current chunk sizes to reduce rehashing.
- **`internal/diff/diff.go` (`Compute`, `warmupAll`)** — `transitionsByKey` uses a **two-pass** build (count then fill). The object loop calls **`Checksum()` once per object** before branching (same semantics as before). **`warmupAll`** on cold layouts uses **`min(GOMAXPROCS(0), N)`** workers over a **`chan *fs.Object`** instead of **N** goroutines plus a semaphore. **`handleChanged`** for tables with transitions sets **`TransitionPaths`** with a **pre-sized `[]string`**.

- **`internal/types/chunk.go` (`BuildINQuery` / `BuildDualINQuery`)** — placeholder lists are written with a **`strings.Builder`** and **`strconv.AppendInt`** into a stack scratch buffer instead of **`fmt.Sprintf` + `strings.Join` over `[]string`**, cutting CPU and allocations on large `IN` lists; the **`[]any` args slice** is unchanged (one interface box per key remains). **`ChunkKeys`** allocates a pre-sized `[][]string` and assigns chunk views instead of **`append`**.

- **`internal/fs/scanner.go` (`preloadGitInfo`, batched `git log` path)** — parses command output with a **newline walk over the raw `[]byte`**, avoiding a full-buffer **`string` copy** and a **`[]string` line slice** from `strings.Split`. **`parseBatchedGitLogCommitLine`** parses `COMMIT|%H|%an|%aI` without **`bytes.SplitN`**. A **`wantedRel`** set of **`filepath.ToSlash(Path)`** skips **`filepath.Join`** and **`gitMap` inserts** for paths outside the current `layout`. **`normalizeGitPathBytesInPlace`** flips `\`→`/` without **`bytes.ReplaceAll`**. See **`docs/perf/scanner-preload-gitinfo-plan.md`**.

- **`internal/bus/bus.go` (`Publish`)** — **`invokeBusHandler`** wraps **`recover`** around each subscriber without allocating a **new closure per handler** in the publish loop.

- **`internal/db/inspector_impl.go` (`scopeKey`)** — builds the sorted part list in a **pre-sized `[]string`** with **indexed writes** (no **`append`**); an **empty layout** returns **`""`** immediately.

Further candidates (not implemented): none tracked here; re-open in `docs/internal-performance-audit.md` if a new hotspot appears.

## References

- `docs/profiling-benchmark-plan.md`
- `docs/internal-performance-audit.md`
