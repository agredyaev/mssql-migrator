# Technical Document: `Scanner.preloadGitInfo` (batched `git log`) plan

Lifecycle: `Current`.

## Purpose

This document is the **canonical plan** for the fast batched path in `(*Scanner).preloadGitInfo`: what it optimizes, what is already implemented, how to measure it, and what to try next. It exists so `BenchmarkScannerPreloadGitInfo_*` results are interpretable without chat history.

## Scope

- Code: `internal/fs/scanner.go` (`preloadGitInfo`, `parseBatchedGitLogCommitLine`, `normalizeGitPathBytesInPlace`, `applyPreloadedGitInfo`)
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

and parses stdout to map **absolute paths** → last commit metadata, then applies that map to `layout.Objects`, `layout.Transitions`, and `layout.Checks`.

## Interfaces And Boundaries

- Inputs: repository `root` string, `*Layout` with `Path` / `AbsPath` populated like production `Scan`.
- Outputs: side effect only — `CachedFile.preloadGitInfo` filled where paths match.
- Fallback: if `git` fails, per-file `GitInfo` calls run in parallel (unchanged).

## Assumptions And Constraints

- Assumptions: `git log --name-only` paths are relative to `root` and match `filepath.ToSlash(layout.*.Path)` after normalizing Windows separators in the log line.
- Constraints: skipping unknown paths **must not** drop metadata for any path that appears in the layout; correctness is proven by `go test ./internal/fs` and integration coverage that exercises `Scan`.

## Nominal Flow (batched path, current)

1. Run `git log` once; read `[]byte` stdout.
2. Build `wantedRel`: set of `filepath.ToSlash(Path)` for every object, transition, and check in `layout`.
3. Scan stdout line-by-line (no `strings.Split` of the full buffer).
4. `COMMIT|…` lines update current commit metadata (`parseBatchedGitLogCommitLine`).
5. Other non-empty lines: normalize `\` → `/` **in place** on the line slice, `rel := string(line)`; if `rel` ∉ `wantedRel`, **continue** (skip `filepath.Join` and map insert for unrelated repo files).
6. Otherwise `abs := filepath.Join(root, rel)`; first-seen wins in `gitMap`.
7. `applyPreloadedGitInfo` walks layout slices and calls `preloadGitInfo` on hits.

## Measured spot checks (`darwin/arm64`, `Apple M2`)

- **`BenchmarkScannerPreloadGitInfo_200Paths`** (`-benchtime=150ms`, `-count=3`): **~174–181 ms/op** when only **one** timed iteration completes per pass (dominated by `git`); **~356 KiB/op**, **~2025 allocs/op** (includes `wantedRel` for **200** paths).
- **`BenchmarkScannerPreloadGitInfo_200Paths_5kExtraGitFiles`** (`-benchtime=200ms`, `-count=2`): **~1.5–2.0 s/op** at `b.N=1`, **~3.4 MiB/op**, **~37k allocs/op** — `git log` lists **~5200** paths while the layout still references **200** SQL files; `wantedRel` avoids building `gitMap` entries for the **~5000** noise paths.

Re-run on your machine before claiming regressions; subprocess noise dominates `ns/op`.

## Phased work (status)

### Phase A — Done (parser + footprint)

- Newline walk over raw `git log` bytes (no `strings.Split(string(out), "\n")`).
- `parseBatchedGitLogCommitLine` without `bytes.SplitN` on commit rows.
- `gitMap` capacity hint from layout size (lower bound).

### Phase B — Done (layout subset filter)

- `wantedRel` set + early `continue` for paths not in the layout.
- In-place slash normalization (`normalizeGitPathBytesInPlace`) instead of `bytes.ReplaceAll` per line.
- `gitMap` sized to `len(wantedRel)` (upper bound on useful entries).
- `applyPreloadedGitInfo` helper for the three layout passes.

**Why it matters:** on large repositories, `git log --name-only` can list far more paths than the SQL layout contains; skipping early reduces **map writes**, **join allocations**, and **CPU** on those lines.

### Phase C — Optional next ideas (measure before coding)

- **Single layout pass** to build `wantedRel` and a flat `[]*CachedFile` list for apply — micro-optimization; only if profiles show the triple loop matters.
- **`benchstat`** on `BenchmarkScannerPreloadGitInfo_200Paths_5kExtraGitFiles` before vs after Phase B to capture allocator deltas (`n≥5`).
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
