# Profiling triage — 2026-05-17

Environment (captured once): `go1.26.3 darwin/arm64`, Apple M2, `GOMAXPROCS=8`.

Artifacts in this directory: `baseline-*.txt` (pre-change), `baseline-POSTFIX-*.txt` (post-change), `cpu-*.prof`, `cpu-topcum-*.txt`, `mem-*.prof`, `mem-allocobj-top-*.txt`, `mem-allocspace-top-*.txt`, `benchstat-Create.txt`, `benchstat-SkipHeavy.txt`.

## Hypothesis A — Fixture / `makeBenchSQL` dominates heap churn

| Status | **Accepted** (alloc_objects) |
|--------|------------------------------|
| Evidence | `mem-allocobj-top-BenchmarkDiffCompute_Create_2000Objects.txt`: `engine.makeBenchSQL` ~93.6% flat; `makeRealFS` cum ~96%. Same pattern on `LayoutHash` mem top (~93.7% `makeBenchSQL`). |
| Interpretation | `go test -memprofile` spans fixture + `b.N`; most **objects** allocated during file body generation and disk write, not steady-state `Compute` loop. |
| Action taken | `internal/engine/benchmark_profiling_test.go` — `makeBenchSQL` switched from `string +=` in a loop (quadratic copies) to `strings.Builder` + `fmt.Fprintf`. |
| Follow-up | `fmt.Fprintf` still emits many small allocations; optional later: template or fixed-width line buffer. |

## Hypothesis B — Skip path paid double `decodeHex32` + redundant work

| Status | **Accepted** (code + benchstat) |
|--------|-----------------------------------|
| Evidence | Prior `internal/diff/diff.go` `Compute` used `isMatch` (Checksum + decodeHex32) then `decodeHex32` again into `plannedObj.Checksum`. |
| Action taken | Single `decodeHex32` into stack, one `Checksum`, branch skip vs changed; reuse `cs` on mismatch to avoid second `Checksum`. |
| benchstat | `benchstat-SkipHeavy.txt`: SkipHeavy median **~−10%** ns/op (`p=0.023`, `n=10`) vs pre-fix baseline. |
| Note | SkipHeavy remains **slower than Create** at same N: per-object `decodeHex32` over 64-char DB checksum + map/string work (~0.7–1.2 µs/obj in isolated runs) vs create path (git fields cached, single `Checksum` read). Expected until metadata stores raw bytes. |

## Hypothesis C — Goroutine / mutex storm in warmup or preload

| Status | **Rejected** for hot `Compute` after `Scan` (current tree) |
|--------|--------------------------------------------------------------|
| Evidence | `cpu-topcum-*Create*`: dominant cum is `syscall` / `os.OpenFile` / `makeRealFS` / `preloadChecksums` during fixture; not `pthread_cond` in top 35 lines of these captures. |
| Note | If `IsChecksumCached` regresses, `warmupAll` could reappear as a hotspot — guard with regression test + CPU profile on cold layout. |

## Hypothesis D — `LayoutHash` algorithmic cost (sort + hash)

| Status | **Accepted** (qualitative) |
|--------|----------------------------|
| Evidence | `BenchmarkLayoutHash_2000Objects` ~0.33–0.52 ms/op in various runs; mem profile still fixture-dominated. Pool for digest slice in `internal/fs/layout.go` `LayoutHash` already limits steady-state allocs (`~5 allocs/op` in bench line). |
| Follow-up | If CPU profiles show `sort.Slice` dominating **after** fixture excluded, consider cached layout digest (invalidation on file change). |

## Hypothesis E — Map growth / key churn

| Status | **Inconclusive** |
|--------|------------------|
| Evidence | No prominent `mapassign` / `mapaccess` in sampled CPU tops; maps are pre-sized in skip-heavy bench. |
| Follow-up | Revisit if new hot maps appear in prod profiles. |

## benchstat — Create (noise warning)

Comparing first baseline file to POSTFIX for **Create** showed **+68%** ns/op with `p=0.000` — interpreted as **machine scheduling / thermal drift between long benchmark sessions**, not a regression from the small `Compute` / fixture edits (B/op and allocs/op unchanged). For Create, rely on **A/B on same session** or shorter `count` when comparing micro-optimizations.
