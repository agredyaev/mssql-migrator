# Technical Document: OPENJSON inspector validation run

Lifecycle: `Current`.

## What this is

This document records the first validation run for the `OPENJSON`-based inspector read path added in `internal/db/inspector_impl.go` and `internal/db/sql/*_openjson.sql`.

## Why it exists

The inspector previously paid most of its local benchmark cost in chunked `IN (...)` query shaping. The new path keeps the old chunked SQL as a fallback, but uses static `OPENJSON` SQL plus a cached compatibility probe when the target database supports compatibility level `130+`.

This document is the durable evidence for:

- what changed,
- how to reproduce the measurements,
- what the local micro-benchmark actually measured,
- what was not validated on this machine.

## Scope

- Implementation:
  - `internal/db/inspector_impl.go`
  - `internal/db/sql/schemas_openjson.sql`
  - `internal/db/sql/objects_openjson.sql`
  - `internal/db/sql/columns_openjson.sql`
  - `internal/db/sql/openjson_compatibility.sql`
- Unit coverage:
  - `internal/db/inspector_test.go`
- Integration coverage:
  - `internal/db/inspector_integration_test.go`
- Bench harness:
  - `internal/db/inspector_bench_test.go`

Out of scope:

- end-to-end migration wall-clock on a live SQL Server,
- TDS driver internals below `database/sql`,
- unsupported-database fallback performance (that path remains covered by unit tests).

## How it works

1. `(*inspector).Inspect()` probes `sys.databases.compatibility_level` once per inspector instance.
2. If the current database reports `>= 130`, inspector marshals schema or object name slices to JSON once per call and runs static `OPENJSON(@pN)` SQL.
3. If the probe reports unsupported compatibility, inspector keeps the historical `BuildINQuery` / `BuildDualINQuery` chunked path.
4. The benchmark harness in `internal/db/inspector_bench_test.go` uses a `MockConn` that explicitly returns `openjson_supported = 1`, so the saved numbers below measure the intended SQL2019-capable path rather than a probe miss.

## Validation commands

From the repository root:

```bash
go test ./internal/db -count=1

go test ./internal/db -run '^$' \
  -bench '^BenchmarkInspectorInspect_Cold_500Objects$|^BenchmarkInspectorInspect_HotCache_500Objects$' \
  -benchmem -count=3 -benchtime=200ms

go test ./internal/db -run '^$' \
  -bench '^BenchmarkInspectorInspect_Cold_500Objects$' \
  -count=1 -benchtime=400ms \
  -memprofile=/tmp/rmig-pprof-20260518/db-openjson.mem.prof

go test ./...
```

Integration test when Docker and local SQL Server are available:

```bash
docker compose up -d

RMIG_RUN_SQLSERVER_INTEGRATION=1 \
RM_DB_SERVER=localhost \
RM_DB_PORT=1433 \
RM_DB_DATABASE=dactests \
RM_DB_USER=sa \
RM_DB_PASSWORD='yourStrong(!)Password' \
go test -tags=integration ./internal/db -run '^TestIntegration_Inspect_UsesOpenJSON$' -count=1
```

## Measured result

Environment:

- `goos: darwin`
- `goarch: arm64`
- `cpu: Apple M2`
- date: `2026-05-18`

Benchmark output range (`-count=3`, `-benchtime=200ms`):

| Benchmark | Time/op | B/op | Allocs/op |
| --- | --- | --- | --- |
| `BenchmarkInspectorInspect_Cold_500Objects` | `95.3-98.8 us/op` | `193163-193378 B/op` | `50 allocs/op` |
| `BenchmarkInspectorInspect_HotCache_500Objects` | `22.6-23.8 us/op` | `25952 B/op` | `5 allocs/op` |

Comparison against the earlier local audit capture recorded in `docs/internal-performance-audit.md`:

- Cold inspect improved from roughly `123-143 us/op` to roughly `95-99 us/op`.
- Cold inspect heap dropped from roughly `203 KiB/op` to roughly `193 KiB/op`.
- Cold inspect alloc count increased from roughly `45 allocs/op` to `50 allocs/op` because the new path adds a compatibility probe plus JSON marshaling work in the benchmark harness.
- Hot-cache inspect stayed in the same band and is slightly faster on this capture (`~23 us/op` vs `~24-26 us/op` previously).

## What this does not prove

- These numbers use `internal/testutil.MockConn`, so they measure Go-side query shaping and inspector cache behavior, not real SQL Server round-trip latency.
- The local integration test was added, but it was not executed in this run because Docker was unavailable on the machine (`/var/run/docker.sock` was not reachable).
- This run does not claim that every real database will prefer `OPENJSON`; the fallback path still exists for compatibility levels below `130`.

## References

- `docs/internal-performance-audit.md`
- `docs/perf/README.md`
- `docs/profiling-benchmark-plan.md`
- `internal/db/inspector_impl.go`
- `internal/db/inspector_bench_test.go`
- `internal/db/inspector_test.go`
- `internal/db/inspector_integration_test.go`
