# Performance run: apply + layout path indexes (memory and CPU profiles)

## What this is

Checked-in **benchmark stdout**, **`pprof` memory profiles** (small binary blobs), and **frozen `go tool pprof -text` summaries** for allocator- and SQL-path work in `internal/apply` and path-index work in `internal/fs`.

## Why it exists

Gives **reproducible profiling evidence** (not only `go test -benchmem` lines) for reviewers and future regressions: where allocations still dominate (`alloc_objects`), and where CPU time goes for the same harness.

## How it was produced

- **Machine:** `darwin/arm64`, `cpu: Apple M2` (from `go test` headers in the `bench-*-run-*.txt` files).
- **Go:** whatever `go version` was on the machine at capture time (re-run commands below on your toolchain).

### Memory (`alloc_objects`)

1. Build a test binary (symbols for `go tool pprof`):

   ```bash
   go test -c -o /tmp/apply.test ./internal/apply
   go test -c -o /tmp/fs.test ./internal/fs
   ```

2. Record profiles into this directory (example paths):

   ```bash
   DOC=docs/perf/runs/2026-05-18-apply-fs-profile
   go test ./internal/apply -run '^$' -bench '^BenchmarkCollectStatements_500Transactional$' \
     -count=1 -benchtime=400ms -memprofile="$DOC/mem-BenchmarkCollectStatements_500Transactional.prof"
   go test ./internal/apply -run '^$' -bench '^BenchmarkExecuteTxBatch_FailurePath_100Statements$' \
     -count=1 -benchtime=400ms -memprofile="$DOC/mem-BenchmarkExecuteTxBatch_FailurePath_100Statements.prof"
   go test ./internal/fs -run '^$' -bench '^BenchmarkScannerPreloadGitInfo_200Paths$' \
     -count=1 -benchtime=300ms -memprofile="$DOC/mem-BenchmarkScannerPreloadGitInfo_200Paths.prof"
   go test ./internal/fs -run '^$' -bench '^BenchmarkLayoutRebuildPathIndexes_500Objects$' \
     -count=1 -benchtime=200ms -memprofile="$DOC/mem-BenchmarkLayoutRebuildPathIndexes_500Objects.prof"
   ```

3. Summarize allocation counts:

   ```bash
   go tool pprof -alloc_objects -top -nodecount=35 /tmp/apply.test "$DOC/mem-BenchmarkCollectStatements_500Transactional.prof"
   go tool pprof -alloc_objects -top -nodecount=35 /tmp/apply.test "$DOC/mem-BenchmarkExecuteTxBatch_FailurePath_100Statements.prof"
   go tool pprof -alloc_objects -top -nodecount=35 /tmp/fs.test "$DOC/mem-BenchmarkScannerPreloadGitInfo_200Paths.prof"
   go tool pprof -alloc_objects -top -nodecount=30 /tmp/fs.test "$DOC/mem-BenchmarkLayoutRebuildPathIndexes_500Objects.prof"
   ```

Interactive (local only):

```bash
go tool pprof -http=:0 /tmp/apply.test "$DOC/mem-BenchmarkCollectStatements_500Transactional.prof"
```

### CPU (optional)

```bash
DOC=docs/perf/runs/2026-05-18-apply-fs-profile
go test ./internal/apply -run '^$' -bench '^BenchmarkCollectStatements_500Transactional$' \
  -count=1 -benchtime=300ms -cpuprofile="$DOC/cpu-BenchmarkCollectStatements_500Transactional.prof"
go test -c -o /tmp/apply-cpu.test ./internal/apply
go tool pprof -top -cum -nodecount=25 /tmp/apply-cpu.test "$DOC/cpu-BenchmarkCollectStatements_500Transactional.prof"
```

## How to interpret the captures

| Artifact | Main signal |
| --- | --- |
| `mem-allocobj-top-BenchmarkCollectStatements_500Transactional.txt` | `encoding/hex.EncodeToString` and `(*Executor).collectStatements` dominate `alloc_objects` (~97% in the captured run); next steps for fewer allocs would target checksum string materialization, not map sizing. |
| `mem-allocobj-top-BenchmarkExecuteTxBatch-Failure.txt` | `newObjectEvent`, `MockConn.ExecContext`, and `buildSingleTxSQL` / `strings.(*Builder).Grow` show where failure-path work allocates. |
| `mem-allocobj-top-BenchmarkScannerPreloadGitInfo_200Paths.txt` | Git / `os/exec` and filesystem helpers dominate; not the path-index maps. |
| `mem-allocobj-top-BenchmarkLayoutRebuildPathIndexes_500Objects.txt` | `buildObjectsByPath` + `buildTransitionsByPath` account for the steady-state map rebuild allocs in isolation. |
| `cpu-topcum-BenchmarkCollectStatements_500Transactional.txt` | CPU sample includes benchmark harness and GC; still shows `collectStatements` in cumulative top under load. |

## Files in this directory

| File | Role |
| --- | --- |
| `bench-mem-run-*.txt` | Raw `go test -benchmem` lines for the profiled benchmark. |
| `bench-cpu-run-*.txt` | Raw bench output for the CPU profile. |
| `mem-*.prof` / `cpu-*.prof` | Binary profiles for `go tool pprof`. |
| `mem-allocobj-top-*.txt` / `cpu-topcum-*.txt` | Text snapshots generated with `go tool pprof`. |

## What this does not cover

- End-to-end MSSQL apply latency or live engine runs (no database in these benches).
- Linux vs Windows profile shapes (paths and syscall stacks differ).

## References

- `internal/apply/apply_bench_test.go`
- `internal/fs/layout_bench_test.go`
- `internal/fs/scanner_bench_test.go`
- `docs/internal-performance-audit.md`
- `docs/profiling-benchmark-plan.md`
