# Performance evidence index (`docs/perf/`)

## What this is

A single entry point for **benchmark deltas**, **`pprof` captures**, and links into the performance audit. If you expected a large wall-clock speedup on real migrations and do not see it, read **“What did not get faster”** below first.

## Where the numbers are

| Question | Document / artifact |
| --- | --- |
| **Before vs after (apply benches)** | `runs/2026-05-18-baseline-vs-post/README.md` — start with the **TL;DR** table (memory and allocs are the stable wins). Raw `benchstat`: `runs/2026-05-18-baseline-vs-post/benchstat-apply-only.txt`. |
| **Where allocations still go (`pprof`)** | `runs/2026-05-18-apply-fs-profile/README.md` and `mem-allocobj-top-*.txt` in that folder. |
| **`preloadGitInfo` (`git log` batch path)** | `runs/2026-05-18-scanner-preload-gitinfo-profile/README.md` — steady-state **`alloc_objects`** / CPU for `BenchmarkScannerPreloadGitInfo_200Paths`, plus short-window and **5k-noise** captures (read the README before interpreting `b.N==1` profiles). |
| **`OPENJSON` inspector read path** | `runs/2026-05-18-openjson-inspector/README.md` — cached compatibility probe, static `OPENJSON` SQL, unit and integration coverage paths, and post-change `BenchmarkInspectorInspect_*` numbers. |
| **Cold object byte cache + `RebuildPathIndexes` retain metadata** | `runs/2026-05-18-cold-object-profile/README.md` — `pprof` text extracts, `bench-post-validation-ScanHint.txt`, run-to-run **`benchstat`**: `benchstat-scanhint-run1-vs-run2.txt`. |
| **`fieldalignment -fix` + post-fix profiles** | `runs/2026-05-18-fieldalignment-after/README.md` — commands, manual literal fixes, `cpu-top-*` / `mem-allocobj-*` text extracts. |
| **Profile rerun (same benches as fieldalignment-after)** | `runs/2026-05-18-profile-rerun/README.md` — refreshed `bench-*` and `pprof` text extracts; raw `*.prof` in `/tmp/prof-rerun-*.prof`. |
| **Fuzz smoke + Phase 3 scopeKey / dual-IN profiles** | `runs/2026-05-19-fuzz-phase3-profile/README.md` — fuzz stdout (`fuzz-*.txt`), benches + `cpu-top-*` / `mem-allocobj-*` (includes **ScopeKey phase3**, **BuildDualINQuery**). |
| **Full audit bench log (historical)** | `runs/2026-05-18-audit-bench/bench-output.txt`. |
| **Narrative audit** | `../internal-performance-audit.md` (repo root `docs/`). |
| **`scopeKey` / Phase 3 inspector cache slot key** | `scopekey-optimization-plan.md` — canonical `scopeKey` + **SHA-256 hex** digest for `Inspect` map keys; verification commands and benchmarks. |
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

- **`internal/types/chunk.go` (`BuildINQuery` / `BuildDualINQuery`)** — placeholder lists are written with a **`strings.Builder`** and **`strconv.AppendInt`** into a stack scratch buffer instead of **`fmt.Sprintf` + `strings.Join` over `[]string`**. `BuildDualINQuery` emits both placeholder sets in **one pass over the SQL template**, so repeated `{{schema_list}}` / `{{object_list}}` expansions no longer pay for chained whole-query `strings.Replace` copies. These builders now return **`[]string`** args, and `internal/driver.Conn.QueryStringsContext` pushes **string→`any` boxing** down to the concrete driver boundary. `internal/db/inspector_impl.go` keeps that chunked path as the compatibility fallback, but the preferred path now uses a **cached `OPENJSON` compatibility probe**, static `internal/db/sql/*_openjson.sql` text, and one JSON marshal per schema or object name set instead of nested `IN (...)` chunk loops on SQL Server `compatibility_level >= 130`. **`ChunkKeys`** still allocates a pre-sized `[][]string` and assigns chunk views instead of **`append`**.

- **`internal/fs/scanner.go` (`preloadGitInfo`, batched `git log` path)** — parses command output with a **newline walk over the raw `[]byte`**, avoiding a full-buffer **`string` copy** and a **`[]string` line slice** from `strings.Split`. **`parseBatchedGitLogCommitLine`** parses `COMMIT|%H|%an|%aI` without **`bytes.SplitN`**. The current fast path builds one **`Path` → `*CachedFile`** target map, normalizes `\`→`/` in place, preloads metadata on the **first** match, deletes fulfilled targets, and stops parsing once all layout paths are filled. `Scanner` now also keeps a **repo-state keyed Git preload cache** so repeated scans of the same repository tree can skip `git log`, and it skips the old per-file fallback entirely when `sqlRoot` is outside any git repo. See **`docs/perf/scanner-preload-gitinfo-plan.md`**.

- **`internal/fs/scanner.go` (`preloadChecksums`) + `internal/fs/layout.go` (`Content` / `Checksum`)** — checksum preload now uses a **bounded worker pool** plus a **shared checksum cache** keyed by `AbsPath + size + mtime`, so repeated scans can reuse checksum results without reopening or rereading the same files. The eager preload is now **object-only** because `diff.Compute` compares object digests in its hot path; transition/check digests stay lazy until a real execution path needs them. `CachedFile.Content()` still keeps the `os.ReadFile` buffer and aliases it into a `string`, but `CachedFile.Checksum()` no longer routes through `Content()` when checksum is the only thing needed. It reads bytes and hashes directly, which avoids populating the content cache on checksum-only paths and removed a large chunk of cold file-open churn from repeated apply profiling.

- **`internal/audit/load.go` (`LoadChecksums`)** — checksum lookup now prefers one static **`OPENJSON`** query and caches the result map in-process until `audit.Subscriber` writes new history rows for the same connection. That removes repeated large `IN (...)` parameter packs and avoids re-querying unchanged history on repeated plan/apply cycles in the same process.

- **`internal/fs/scanner.go` (`Scanner.Scan`)** — repeated scans now keep a **layout metadata cache per root**, and that snapshot is shared **across `Scanner` instances in the same process**. The cache stores the discovered schema/object/check/transition structure plus directory/file stat snapshots. When those stats still match, `Scan` rebuilds a fresh `Layout` from cached metadata instead of walking the tree with `ReadDir` again, and it pre-seeds per-file checksums from the validated snapshot so repeated plan/apply cycles do not reopen unchanged SQL files just to warm `diff`. The same snapshot now also carries preloaded Git metadata and validates it against the repository state, so cached scans can skip the old `preloadGitInfo` work too. Cache invalidation is conservative: any directory shape change, SQL file stat change, transition file rewrite, or Git repo state change forces a full rescan.

- **`internal/db/inspector_impl.go` (`Inspect`)** — inspector state is now cached **process-wide per `conn + scope + generation`**, not just per `Inspector` instance. That means repeated flows that construct new inspectors still reuse DB inspection results until an apply changes the database. `internal/apply/apply.go` bumps the generation after successful changes so stale schema/object/column state is never reused after a real apply.

- **`internal/bus/bus.go` (`Publish`)** — **`invokeBusHandler`** wraps **`recover`** around each subscriber without allocating a **new closure per handler** in the publish loop.

- **`internal/audit/subscriber.go` + `internal/apply/apply.go`** — the hot transactional success path now publishes one **`[]*types.ObjectEvent`** batch per applied SQL batch, and `audit.Subscriber` turns that into one multi-row `INSERT ... VALUES (...), (...)` call instead of one `ExecContext` per object. That removes the earlier per-object MSSQL RPC parameter churn (`go-mssqldb.(*Stmt).makeRPCParams`) from the top allocation sites in the local `StressApply` profile.

- **`internal/db/inspector_impl.go` (`scopeKey`, `scopeKeySHA256Hex`)** — builds a pre-sized **`[]scopePart`** (`{kind byte, s string}`), sorts by tuple order equivalent to legacy `"kind:payload"` strings, and emits once via **`strings.Builder`**; **`Inspect`** keys caches by **`hex(SHA256(UTF-8(canonical)))`** (or `""` when canonical is empty). An **empty layout** returns canonical **`""`** immediately.

Further candidates (not implemented): none tracked here; re-open in `docs/internal-performance-audit.md` if a new hotspot appears.

## References

- `docs/profiling-benchmark-plan.md`
- `docs/internal-performance-audit.md`
