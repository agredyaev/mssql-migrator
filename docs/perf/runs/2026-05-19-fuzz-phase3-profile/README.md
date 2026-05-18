# Fuzz smoke + Phase 3 scopeKey profiles (2026-05-19)

## What this is

- **Fuzz:** 3s smoke per target (`internal/db`, `internal/types`); stdout saved as `fuzz-*.txt`.
- **Profiles:** same benchmark + `pprof` pattern as `runs/2026-05-18-profile-rerun`, plus **`BenchmarkScopeKeyPhase3SlotKey_2000Parts`** (canonical `scopeKey` + `scopeKeySHA256Hex`) and **`BenchmarkBuildDualINQuery_500x500`** (`internal/types`).

## Commands

```bash
cd /path/to/mssql-reporting-migrator

# Fuzz (examples; increase -fuzztime in CI or nightly)
go test ./internal/db -fuzz=FuzzScopeKey_stringStableUnderRepeat -fuzztime=3s
go test ./internal/db -fuzz=FuzzScopeKey_digestMatchesSHA256 -fuzztime=3s
go test ./internal/types -fuzz=FuzzNormalizedKey_matchesConcatLower -fuzztime=3s
go test ./internal/types -fuzz=FuzzBuildDualINQuery_expandsPlaceholders -fuzztime=3s

# Symbolized profiles (binaries under /tmp; *.prof in /tmp — gitignored)
go test -c -o /tmp/fs-bench.test ./internal/fs
go test -c -o /tmp/apply.test ./internal/apply
go test -c -o /tmp/db-bench.test ./internal/db
go test -c -o /tmp/types-bench.test ./internal/types

DOC=docs/perf/runs/2026-05-19-fuzz-phase3-profile
go test ./internal/fs -run '^$' -bench '^BenchmarkObjectChecksumSharedBytesAfterScan/withScanHint$' \
  -cpuprofile=/tmp/fuzzpf-cpu-scanhint.prof -memprofile=/tmp/fuzzpf-mem-scanhint.prof -benchmem -benchtime=3s -count=1
go test ./internal/fs -run '^$' -bench '^BenchmarkLayoutRebuildPathIndexes_500Objects$' \
  -memprofile=/tmp/fuzzpf-mem-layout.prof -benchmem -benchtime=3s -count=1
go test ./internal/db -run '^$' -bench '^BenchmarkScopeKey_2000Parts$' \
  -cpuprofile=/tmp/fuzzpf-cpu-scopekey.prof -memprofile=/tmp/fuzzpf-mem-scopekey.prof -benchmem -benchtime=2s -count=1
go test ./internal/db -run '^$' -bench '^BenchmarkScopeKeyPhase3SlotKey_2000Parts$' \
  -cpuprofile=/tmp/fuzzpf-cpu-scopekey-phase3.prof -memprofile=/tmp/fuzzpf-mem-scopekey-phase3.prof -benchmem -benchtime=2s -count=1
go test ./internal/apply -run '^$' -bench '^BenchmarkCollectStatements_500Transactional$' \
  -memprofile=/tmp/fuzzpf-mem-collect.prof -benchmem -benchtime=400ms -count=1
go test ./internal/types -run '^$' -bench '^BenchmarkBuildDualINQuery_500x500$' \
  -memprofile=/tmp/fuzzpf-mem-dualin.prof -benchmem -benchtime=1s -count=1

go tool pprof -top -nodecount=25 /tmp/fs-bench.test /tmp/fuzzpf-cpu-scanhint.prof
go tool pprof -top -alloc_objects -nodecount=25 /tmp/fs-bench.test /tmp/fuzzpf-mem-scanhint.prof
go tool pprof -top -alloc_objects -nodecount=30 /tmp/fs-bench.test /tmp/fuzzpf-mem-layout.prof
go tool pprof -top -nodecount=25 /tmp/db-bench.test /tmp/fuzzpf-cpu-scopekey.prof
go tool pprof -top -alloc_objects -nodecount=25 /tmp/db-bench.test /tmp/fuzzpf-mem-scopekey.prof
go tool pprof -top -nodecount=25 /tmp/db-bench.test /tmp/fuzzpf-cpu-scopekey-phase3.prof
go tool pprof -top -alloc_objects -nodecount=25 /tmp/db-bench.test /tmp/fuzzpf-mem-scopekey-phase3.prof
go tool pprof -top -alloc_objects -nodecount=25 /tmp/apply.test /tmp/fuzzpf-mem-collect.prof
go tool pprof -top -alloc_objects -nodecount=25 /tmp/types-bench.test /tmp/fuzzpf-mem-dualin.prof
```

## Checked-in artifacts

| File | Contents |
| --- | --- |
| `fuzz-*.txt` | Fuzz worker summary (3s each) |
| `bench-*.txt` | `go test -bench` stdout |
| `cpu-top-*.txt` / `mem-allocobj-*.txt` | `go tool pprof -top` extracts |

## How it is validated

- `go test ./...` — green before captures.
- Fuzz targets completed with `PASS` for the chosen `-fuzztime`.

## What it does not cover

- Long fuzz campaigns (hours); use larger `-fuzztime` or dedicated job for deeper corpus.
- End-to-end `Inspect` wall time on real SQL Server.

## References

- Fuzz sources: `internal/db/inspector_fuzz_test.go`, `internal/types/types_fuzz_test.go`, `internal/types/chunk_fuzz_test.go`.
- Phase 3: `docs/perf/scopekey-optimization-plan.md`.
