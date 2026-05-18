# Profile rerun (2026-05-18)

## What this is

A second capture of the same benchmark + `pprof` suite as `runs/2026-05-18-fieldalignment-after/README.md` (ScanWithHint, LayoutRebuild, ScopeKey, CollectStatements), after further `fieldalignment` / layout changes, to refresh numbers and text extracts on this machine.

## How it was produced

```bash
cd /path/to/mssql-reporting-migrator
go test -c -o /tmp/fs-bench.test ./internal/fs
go test -c -o /tmp/apply.test ./internal/apply
go test -c -o /tmp/db-bench.test ./internal/db

DOC=docs/perf/runs/2026-05-18-profile-rerun
go test ./internal/fs -run '^$' -bench '^BenchmarkObjectChecksumSharedBytesAfterScan/withScanHint$' \
  -cpuprofile=/tmp/prof-rerun-cpu-scanhint.prof -memprofile=/tmp/prof-rerun-mem-scanhint.prof -benchmem -benchtime=3s -count=1
go test ./internal/fs -run '^$' -bench '^BenchmarkLayoutRebuildPathIndexes_500Objects$' \
  -memprofile=/tmp/prof-rerun-mem-layout.prof -benchmem -benchtime=3s -count=1
go test ./internal/db -run '^$' -bench '^BenchmarkScopeKey_2000Parts$' \
  -cpuprofile=/tmp/prof-rerun-cpu-scopekey.prof -memprofile=/tmp/prof-rerun-mem-scopekey.prof -benchmem -benchtime=2s -count=1
go test ./internal/apply -run '^$' -bench '^BenchmarkCollectStatements_500Transactional$' \
  -memprofile=/tmp/prof-rerun-mem-collect.prof -benchmem -benchtime=400ms -count=1

go tool pprof -top -nodecount=25 /tmp/fs-bench.test /tmp/prof-rerun-cpu-scanhint.prof
go tool pprof -top -alloc_objects -nodecount=25 /tmp/fs-bench.test /tmp/prof-rerun-mem-scanhint.prof
go tool pprof -top -alloc_objects -nodecount=30 /tmp/fs-bench.test /tmp/prof-rerun-mem-layout.prof
go tool pprof -top -alloc_objects -nodecount=25 /tmp/apply.test /tmp/prof-rerun-mem-collect.prof
go tool pprof -top -nodecount=20 /tmp/db-bench.test /tmp/prof-rerun-cpu-scopekey.prof
go tool pprof -top -alloc_objects -nodecount=20 /tmp/db-bench.test /tmp/prof-rerun-mem-scopekey.prof
```

Raw `*.prof` files are under `/tmp/prof-rerun-*.prof` (not checked in; `*.prof` is gitignored).

## Checked-in artifacts

| File | Contents |
| --- | --- |
| `bench-*.txt` | `go test -bench` stdout |
| `cpu-top-ScanWithHint.txt` | CPU top — fs checksum + Scan hint |
| `cpu-top-ScopeKey.txt` | CPU top — `scopeKey` |
| `mem-allocobj-*.txt` | `alloc_objects` top for fs / apply / db |

## How it is validated

- Commands completed successfully; `bench-*.txt` and `*-top-*.txt` were written from the same run.
- For symbolized stacks, the first `pprof` argument must be the matching `*.test` binary built with `go test -c` (see `runs/2026-05-18-scanner-preload-gitinfo-profile/README.md`).

## What it does not cover

- `benchstat` against the earlier `fieldalignment-after` capture (not run here).
- Other benchmarks (git preload, diff, bus, etc.).

## References

- Prior same-suite run: `runs/2026-05-18-fieldalignment-after/README.md`.
- Index: `docs/perf/README.md`.
