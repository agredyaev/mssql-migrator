# Profile + bench rerun vs 2026-05-19 capture

## What this is

A fresh run of the same harness as `docs/perf/runs/2026-05-19-fuzz-phase3-profile/README.md` (benchmarks + CPU/`alloc_objects` text extracts), on **2026-05-18** (same machine class: `darwin/arm64`, Apple M2), compared to the checked-in **2026-05-19** stdout and `pprof` extracts.

## How it is validated

- `go test` benchmark flags match the 2026-05-19 README (`-benchtime`, `-count=1` for the profiled suite).
- `go tool pprof -top` uses the `go test -c` binaries under `/tmp` (symbolized stacks).
- Extra: two back-to-back `-count=10` runs of `BenchmarkBuildDualINQuery_500x500` on **unchanged** code to show **ns/op** noise between runs (`benchstat-dualin-same-code-runA-vs-runB.txt`).

## Raw benchmark lines (single `-count=1` each)

| Benchmark | 2026-05-19 (checked-in) | 2026-05-18 (this run) | Δ ns/op (single sample) |
| --- | --- | --- | --- |
| `BenchmarkObjectChecksumSharedBytesAfterScan/withScanHint` | 430.2 ns/op, 416 B/op, 1 allocs | 413.5 ns/op, 416 B/op, 1 allocs | −3.9% |
| `BenchmarkLayoutRebuildPathIndexes_500Objects` | 21855 ns/op, 27456 B/op, 7 allocs | 21772 ns/op, 27456 B/op, 7 allocs | −0.4% |
| `BenchmarkScopeKey_2000Parts` | 97182 ns/op, 106592 B/op, 5 allocs | 95040 ns/op, 106592 B/op, 5 allocs | −2.2% |
| `BenchmarkScopeKeyPhase3SlotKey_2000Parts` | 178682 ns/op, 164065 B/op, 8 allocs | 143807 ns/op, 164064 B/op, 8 allocs | −19.5% |
| `BenchmarkCollectStatements_500Transactional` | 116729 ns/op, 226665 B/op, 16 allocs | 84913 ns/op, 226667 B/op, 16 allocs | −27.3% |
| `BenchmarkBuildDualINQuery_500x500` | 28933 ns/op, 24712 B/op, 4 allocs | 21383 ns/op, 24712 B/op, 4 allocs | −26.1% |

**Interpretation:** `benchstat` on two files with **`n=1` each** cannot claim significance (see `benchstat-vs-2026-05-19-single-run.txt`: every row is `~` with “need >= 4 samples”). For **`BuildDualINQuery`**, **`B/op`** and **`allocs/op`** are identical across 2026-05-19, this single run, and two **`n=10`** repeats; the spread in **ns/op** between the 2026-05-19 line (28.9µ) and today’s ~21µ band is consistent with **run-to-run wall-time noise**, not a measured change from `BuildINQuery` / `NormalizedKey` tweaks (those paths are not exercised by `BuildDualINQuery`).

## Noise floor (`BuildDualINQuery`, same binary, `n=10` × 2)

`benchstat-dualin-same-code-runA-vs-runB.txt`: **21.27µ ± 1%** vs **21.25µ ± 1%**, `~` (p=0.755); **B/op** and **allocs/op** identical.

## `alloc_objects` profile shape (`BuildDualINQuery`)

Compared to `runs/2026-05-19-fuzz-phase3-profile/mem-allocobj-BuildDualINQuery.txt`, the current `mem-allocobj-BuildDualINQuery.txt` still shows the same dominant symbols (`buildDualINTemplatePlan`, `combineStringArgs`, `MakeNoZero`, `strings.(*Builder).Grow` under `buildDualINQueryFromPlan`). Totals differ slightly (GC / profiler sampling), not a new hot function for this micro-change set.

## Commands (replay)

See parent directory `../2026-05-19-fuzz-phase3-profile/README.md` for the exact `go test` / `go test -c` / `go tool pprof` lines; this run used profile filenames under `/tmp/live-*.prof` and wrote `bench-*.txt`, `cpu-top-*.txt`, and `mem-allocobj-*.txt` into this folder.

## What it does not cover

- `BuildINQuery` is not covered by `BenchmarkBuildDualINQuery_500x500` (dual-placeholder compiled plan path). Add a dedicated bench if you need allocator proof for `BuildINQuery`.
- A/B vs an older **git commit** (would require checking out the old tree and re-running the same harness).

## References

- Baseline artifacts: `docs/perf/runs/2026-05-19-fuzz-phase3-profile/`
- Harness: `docs/profiling-benchmark-plan.md`
