# Performance run: `Scanner.preloadGitInfo` (CPU and heap profiles)

Lifecycle: `Current`.

## Purpose

Checked-in **benchmark stdout** and **frozen `go tool pprof -text` summaries** for `(*Scanner).preloadGitInfo` on the synthetic Git harness in `internal/fs/scanner_bench_test.go`. This answers: where **CPU** and **`alloc_objects`** go relative to **`git`**, **`freshGitBenchLayout`**, and **`filepath.Join`** inside the batched parser path.

## Scope

- Code under test: `internal/fs/scanner.go` (`preloadGitInfo`, batched `git log` parse).
- Benchmarks: `BenchmarkScannerPreloadGitInfo_200Paths`, `BenchmarkScannerPreloadGitInfo_200Paths_5kExtraGitFiles` (`internal/fs/scanner_bench_test.go`).
- Artifacts in this directory: `bench-*.txt`, `mem-allocobj-top-*.txt`, `cpu-topcum-*.txt`.
- Binary `*.prof` files are **not** tracked (see repository `.gitignore` `*.prof`); regenerate locally with the commands below.

Out of scope: live repository wall time, `libgit2`, or end-to-end `rmig` runs.

## System Context

`preloadGitInfo` runs `git log` once, walks stdout bytes, optionally fills a `gitMap`, then applies metadata to `layout` entries. The benchmarks call `preloadGitInfo` in a loop with `b.StopTimer()` around `freshGitBenchLayout` so **`-benchmem`** reflects per-iteration work, but a **heap profile** still records allocations for the **whole** `go test` invocation of that benchmark (including setup when `b.N` is small).

## Interfaces And Boundaries

- Inputs: temporary Git repo root (`b.TempDir()`), `Layout` built like production `Scan` (`freshGitBenchLayout`).
- Outputs: side effects on `CachedFile` git metadata; profiles are observability only.
- Boundary: **`git` on `PATH`** required; profiles include subprocess and stdio buffering.

## Assumptions And Constraints

- Assumptions: capture machine class matches reader expectations (`darwin/arm64`, Apple M2 in the checked-in headers).
- Constraints: default **short** `-benchtime` (for example `400ms` / `1s`) often yields **`b.N == 1`** on the **200-path** bench because the first wall-clock iteration includes **one-time** repo construction inside the benchmark loop (`makeGitBenchRepoWithTrackedViews` on the first `i == 0`). Then **`alloc_objects` / CPU** snapshots are dominated by **fixture**, **`testing.(*B).ResetTimer`**, or **tempdir cleanup**, not steady-state `preloadGitInfo`. The **`*-steady`** captures use **`-benchtime=3s`** so **`b.N` is large** and the profile window weights the hot loop.

## Nominal Flow (how this run was produced)

1. `go test -c -o /tmp/fs-bench.test ./internal/fs` (binary for symbolized `pprof`).
2. `go test ./internal/fs -run '^$' -bench '<name>' -count=1 -benchtime=<duration> [-benchmem] -memprofile=...` and separate runs with `-cpuprofile=...`.
3. `go tool pprof -alloc_objects -top -nodecount=... /tmp/fs-bench.test <mem.prof> > mem-allocobj-top-....txt`
4. `go tool pprof -top -cum -nodecount=... /tmp/fs-bench.test <cpu.prof> > cpu-topcum-....txt`

Capture toolchain: **`go version go1.26.3 darwin/arm64`** (re-run `go version` when regenerating on another machine).

## Off-Nominal Behavior And Failure Containment

- Failure mode: **`go test` fails** (no `git`, sandbox denies `exec`). Containment: fix environment; do not commit partial profiles.
- Failure mode: **`go tool pprof` shows `main.main` only**. Containment: pass the **test binary** (`fs-bench.test`) as the first `pprof` argument, not only the `.prof` file.

## Verification And Validation

- Correctness gate for code changes: `go test ./internal/fs -count=1` (and `go test ./...` before merge).
- Profile regeneration must reproduce **benchmark name regex** and **`-benchtime`** documented in **Files** below.

## How to interpret the captures

### `BenchmarkScannerPreloadGitInfo_200Paths` — steady-state (`*-steady`, `-benchtime=3s`, `b.N≈218`)

From `mem-allocobj-top-BenchmarkScannerPreloadGitInfo_200Paths-steady.txt` (alloc_objects, rounded):

| Approx share of sampled `alloc_objects` | Stack / symbol | Note |
| --- | --- | --- |
| **~37%** | `freshGitBenchLayout` | Outside `StartTimer` in the bench loop; still appears in the process-wide heap profile. |
| **~28%** cum under | `(*Scanner).preloadGitInfo` | Includes **`git`**, **`bytes.Buffer`** growth from `cmd.Output`, **`filepath.Join`**, map work. |
| **~14%** | `path/filepath.join` / `strings.(*Builder).Grow` / `strings.Join` | Consistent with **`filepath.Join(root, rel)`** on matched log lines (200 hits × `b.N`). |

CPU (`cpu-topcum-BenchmarkScannerPreloadGitInfo_200Paths-steady.txt`): a large fraction of samples sits in **runtime scheduler / pthread_cond_wait** and **syscalls** (`syscall.rawsyscalln`, `os.ignoringEINTR`) — expected when **`git` subprocess** dominates wall time; **`preloadGitInfo` ~16% cum** in this capture still shows meaningful in-process work versus **fixture** and **cleanup**.

### `BenchmarkScannerPreloadGitInfo_200Paths` — short window (`400ms`, `b.N==1`)

Treat `mem-allocobj-top-BenchmarkScannerPreloadGitInfo_200Paths.txt` as **fixture-heavy** (for example **`testing.(*B).ResetTimer`** flat, **`makeGitBenchRepoWithTrackedViews`** cum). Prefer **`*-steady`** for allocator attribution on **`preloadGitInfo`**.

### `BenchmarkScannerPreloadGitInfo_200Paths_5kExtraGitFiles` (`-benchtime=1s`, typically `b.N==1`)

The benchmark **creates 5000 files and commits them once**; **`alloc_objects`** and **CPU** are dominated by **`os.WriteFile`**, **`filepath.Join`**, **`strings.Join`**, **`syscall.ByteSliceFromString`**, and **tempdir `RemoveAll` cleanup`** — not the per-line **`preloadGitInfo`** filter alone. Use this capture to reason about **“large `git log` + huge fixture cost”**, not micro-optimizations inside the parser unless you change the harness to move setup **outside** `b.Loop` / raise **`b.N`**.

## Operations And Recovery

Interactive drill-down (local):

```bash
go test -c -o /tmp/fs-bench.test ./internal/fs
DOC=docs/perf/runs/2026-05-18-scanner-preload-gitinfo-profile
go tool pprof -http=:0 /tmp/fs-bench.test "$DOC/mem-BenchmarkScannerPreloadGitInfo_200Paths-steady.prof"
```

Regenerate **steady** text tables after editing `scanner.go`:

```bash
go test -c -o /tmp/fs-bench.test ./internal/fs
DOC=docs/perf/runs/2026-05-18-scanner-preload-gitinfo-profile
go test ./internal/fs -run '^$' -bench '^BenchmarkScannerPreloadGitInfo_200Paths$' \
  -count=1 -benchtime=3s -benchmem \
  -memprofile="$DOC/mem-BenchmarkScannerPreloadGitInfo_200Paths-steady.prof"
go tool pprof -alloc_objects -top -nodecount=45 /tmp/fs-bench.test \
  "$DOC/mem-BenchmarkScannerPreloadGitInfo_200Paths-steady.prof" \
  > "$DOC/mem-allocobj-top-BenchmarkScannerPreloadGitInfo_200Paths-steady.txt"
```

## Files in this directory

| File | Role |
| --- | --- |
| `bench-mem-run-BenchmarkScannerPreloadGitInfo_200Paths.txt` | Short-window benchmem + memprofile driver output (`-benchtime=400ms`). |
| `bench-cpu-run-BenchmarkScannerPreloadGitInfo_200Paths.txt` | Short-window CPU profile driver output. |
| `mem-allocobj-top-BenchmarkScannerPreloadGitInfo_200Paths.txt` | `alloc_objects` / `top` text for short window (fixture-skewed). |
| `cpu-topcum-BenchmarkScannerPreloadGitInfo_200Paths.txt` | CPU `top -cum` for short window. |
| `bench-mem-run-BenchmarkScannerPreloadGitInfo_200Paths-steady.txt` | Steady-state benchmem (`-benchtime=3s`, many iterations). |
| `bench-cpu-run-BenchmarkScannerPreloadGitInfo_200Paths-steady.txt` | Steady-state CPU profile driver output. |
| `mem-allocobj-top-BenchmarkScannerPreloadGitInfo_200Paths-steady.txt` | **`alloc_objects`** top for steady-state (use for `preloadGitInfo` attribution). |
| `cpu-topcum-BenchmarkScannerPreloadGitInfo_200Paths-steady.txt` | CPU `top -cum` for steady-state. |
| `bench-mem-run-BenchmarkScannerPreloadGitInfo_200Paths_5kExtraGitFiles.txt` | 5k-noise harness benchmem (`-benchtime=1s`). |
| `bench-cpu-run-BenchmarkScannerPreloadGitInfo_200Paths_5kExtraGitFiles.txt` | CPU profile driver output for 5k harness. |
| `mem-allocobj-top-BenchmarkScannerPreloadGitInfo_200Paths_5kExtraGitFiles.txt` | `alloc_objects` top (fixture + cleanup heavy). |
| `cpu-topcum-BenchmarkScannerPreloadGitInfo_200Paths_5kExtraGitFiles.txt` | CPU `top -cum` for 5k harness. |
| `mem-*.prof`, `cpu-*.prof` | Local-only binaries (gitignored); paths above match `-memprofile`/`-cpuprofile` targets when regenerated. |

## Open Issues And Non-Goals

- Open issue: the **5k-noise** benchmark should get a **`b.N > 1`** steady profile only if the harness moves **noise creation** out of the timed path or accepts a much longer **`-benchtime`**.
- Non-goals: claiming **ns/op** wins from **`pprof` CPU** rows alone — subprocess **`git`** time dominates; pair with **`-benchmem`** and **`benchstat`**.

## References

- `internal/fs/scanner.go`
- `internal/fs/scanner_bench_test.go`
- `docs/perf/scanner-preload-gitinfo-plan.md`
- `docs/profiling-benchmark-plan.md`
- `docs/internal-performance-audit.md`
- Older related snapshot (200-path mem only): `docs/perf/runs/2026-05-18-apply-fs-profile/README.md`
