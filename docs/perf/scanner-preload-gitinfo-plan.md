# Technical Document: `Scanner.preloadGitInfo` (batched `git log`) plan

Lifecycle: `Current`.

## Purpose

This document is the **canonical plan** for the fast batched path in `(*Scanner).preloadGitInfo`: what it optimizes, what is already implemented, how to measure it, and what still dominates after the current allocator pass. It exists so `BenchmarkScannerPreloadGitInfo_*` results are interpretable without chat history.

## Scope

- Code: `internal/fs/scanner.go` (`preloadGitInfo`, `parseBatchedGitLogCommitLine`, `normalizeGitPathBytesInPlace`, `buildPreloadGitTargets`, repo-state cache helpers)
- Benchmarks: `internal/fs/scanner_bench_test.go` (`BenchmarkScannerPreloadGitInfo_200Paths`, `BenchmarkScannerPreloadGitInfo_200Paths_5kExtraGitFiles`)
- Commands:

```bash
go test ./internal/fs -run '^$' -bench '^BenchmarkScannerPreloadGitInfo_200Paths$' -benchmem -count=5
go test ./internal/fs -run '^$' -bench '^BenchmarkScannerPreloadGitInfo_200Paths_5kExtraGitFiles$' -benchmem -count=3 -benchtime=200ms
```

Out of scope: replacing `git log` with libgit2, async preload, or changing public `Scanner` API without engine impact analysis.

## System Context

After `Scanner.Scan`, `preloadGitInfo` runs when `Scanner.GitInfo` is set. The **fast path** runs:

```text
git -C <root> log --name-only --format=COMMIT|%H|%an|%aI
```

and parses stdout to fill `CachedFile` Git metadata for matching layout paths.

## Interfaces And Boundaries

- Inputs: repository `root` string, `*Layout` with `Path` / `AbsPath` populated like production `Scan`.
- Outputs: side effect only — `CachedFile.preloadGitInfo` filled where paths match.
- Fallback: if `git` fails, per-file `GitInfo` calls run in parallel (unchanged).

## Assumptions And Constraints

- Assumptions: `git log --name-only` paths are relative to `root` and match `filepath.ToSlash(layout.*.Path)` after normalizing Windows separators in the log line.
- Constraints: skipping unknown paths **must not** drop metadata for any path that appears in the layout; correctness is proven by `go test ./internal/fs` and integration coverage that exercises `Scan`.

## Nominal Flow (batched path, current)

1. Run `git log` once; read `[]byte` stdout.
2. Build one `targets` map: `filepath.ToSlash(Path)` → `*CachedFile` for every object, transition, and check in `layout`.
3. If the scanner already has cached Git metadata for the same repository state, preload directly from that cache and skip `git log`.
4. Otherwise run `git log`, scan stdout line-by-line (no `strings.Split` of the full buffer).
5. `COMMIT|…` lines update current commit metadata (`parseBatchedGitLogCommitLine`).
6. Other non-empty lines: normalize `\` → `/` **in place** on the line slice, `rel := string(line)`; if `rel` is not in `targets`, **continue** (skip unrelated repo files early).
7. Otherwise preload the matching `CachedFile` immediately with the current commit metadata, `delete(targets, rel)`, and keep scanning only while unmatched targets remain.
8. Save the matched path metadata in a repo-state keyed cache so later `Scan()` calls on the same tree can reuse it.

## Measured spot checks (`darwin/arm64`, `Apple M2`)

- **Steady `BenchmarkScannerPreloadGitInfo_200Paths`** (`-benchtime=3s`, `-count=1`): **~16.4–16.5 ms/op**, **~76 KiB/op**, **~119 allocs/op** after the direct target-map pass. `alloc_objects` no longer shows the old intermediate `gitMap` / `filepath.Join` heavy shape from the earlier capture.
- **Short-window `BenchmarkScannerPreloadGitInfo_200Paths_5kExtraGitFiles`** (`-benchtime=200ms`, `-count=3`): wall time stays noisy because `git` dominates and `b.N` is still often **1**, but allocator shape moved down to roughly **~3.2 MiB/op** and **~31.6k allocs/op** on the same synthetic repo size.

Re-run on your machine before claiming regressions; subprocess noise dominates `ns/op`.

## Phased work (status)

### Phase A — Done (parser + footprint)

- Newline walk over raw `git log` bytes (no `strings.Split(string(out), "\n")`).
- `parseBatchedGitLogCommitLine` without `bytes.SplitN` on commit rows.
- Capacity hint from layout size (lower bound).

### Phase B — Done (layout subset filter)

- Layout-relative path filter + early `continue` for paths not in the layout.
- In-place slash normalization (`normalizeGitPathBytesInPlace`) instead of `bytes.ReplaceAll` per line.

**Why it matters:** on large repositories, `git log --name-only` can list far more paths than the SQL layout contains; skipping early reduces **map work** and **CPU** on those lines.

### Phase C — Done (direct target preload)

- One `targets` map (`rel path` → `*CachedFile`) replaces the old `wantedRel` + `gitMap` + apply pass.
- Matching lines preload Git metadata immediately on first sight and remove the target from the map.
- The parse loop exits once every layout path has been filled.

**Why it matters:** steady-state `alloc_objects` no longer spends meaningful share on `filepath.Join` and `gitMap` population; the remaining cost is mostly the `git` subprocess, command output buffer, and the benchmark's own `freshGitBenchLayout`.

### Phase D — Done (cross-scan reuse and non-git skip)

- `Scanner` now keeps a repo-state keyed cache of matched Git metadata so repeated `Scan()` calls on the same repository tree can preload without another `git log`.
- The git-dir lookup walks upward from `sqlRoot`, so roots like `.temp/sql` under a checked-out repository still hit the cache.
- When the scan root is outside any git repository, `preloadGitInfo` skips the old per-file fallback entirely because it cannot return metadata there and only burns `os/exec` work; `Scan()` also disables lazy `CachedFile.Git*` lookups for that layout so later planner calls do not retry the same useless `git` process per file.

**Why it matters:** repeated integration or load scans on the same tree no longer need to spawn `git` once the cache is warm, and temp copied layouts outside git avoid useless fallback probes.

### Phase E — Optional next ideas (measure before coding)

- **Zero-copy lookup for `rel := string(line)`** only if profiles show the string conversion itself is now material; current captures still show `git`, DB I/O, or benchmark fixture above it.
- **`benchstat`** on `BenchmarkScannerPreloadGitInfo_200Paths_5kExtraGitFiles` before vs after Phase C to capture allocator deltas (`n≥5`).
- **Path edge cases** (quoted paths, unusual `core.quotePath` settings): add tests if real repos hit mismatches.

## Verification And Validation

- `go test ./internal/fs -count=1`
- `go test ./internal/fs -run '^$' -bench '^BenchmarkScannerPreloadGitInfo' -benchmem -count=5`
- Contract: `internal/fs/fs_test.go` (`TestNormalizeGitPathBytesInPlace`)
- CPU / heap evidence: `docs/perf/runs/2026-05-18-scanner-preload-gitinfo-profile/README.md` (steady-state **`alloc_objects`** for `BenchmarkScannerPreloadGitInfo_200Paths` uses `-benchtime=3s`; read the README before interpreting **`b.N==1`** captures)

## Open Issues And Non-Goals

- Open issues: micro-benches are dominated by **`git` subprocess** wall time; treat **`B/op` / `allocs/op`** as the stable signals unless you pin CPU frequency and increase `-benchtime`.
- Non-goals: proving end-to-end `rmig` speedup from this path alone (DB I/O dominates real runs).

## References

- `internal/fs/scanner.go`
- `internal/fs/scanner_bench_test.go`
- Index: `docs/perf/README.md`
- Audit: `docs/internal-performance-audit.md`
