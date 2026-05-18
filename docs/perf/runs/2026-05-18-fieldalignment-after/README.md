# Post–`fieldalignment -fix` run (2026-05-18)

## What this is

Record of `golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment -fix ./...` on this repository, manual fixes where `-fix` broke keyed/positional composite literals, `go test ./...`, and a small set of **CPU / `alloc_objects`** text extracts from `go tool pprof`.

## Commands

```bash
# Field alignment (reorders struct fields for smaller structs / less padding)
go install golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@latest
fieldalignment -fix ./...

# Tests
go test ./...

# Symbolized profiles (binary *.prof stay in /tmp — gitignored)
go test -c -o /tmp/fs-bench.test ./internal/fs
go test -c -o /tmp/apply.test ./internal/apply
go test -c -o /tmp/db-bench.test ./internal/db

DOC=docs/perf/runs/2026-05-18-fieldalignment-after
go test ./internal/fs -run '^$' -bench '^BenchmarkObjectChecksumSharedBytesAfterScan/withScanHint$' \
  -cpuprofile=/tmp/fa-cpu-scanhint.prof -memprofile=/tmp/fa-mem-scanhint.prof -benchmem -benchtime=3s -count=1
go test ./internal/fs -run '^$' -bench '^BenchmarkLayoutRebuildPathIndexes_500Objects$' \
  -memprofile=/tmp/fa-mem-layout.prof -benchmem -benchtime=3s -count=1
go test ./internal/db -run '^$' -bench '^BenchmarkScopeKey_2000Parts$' \
  -cpuprofile=/tmp/fa-cpu-scopekey.prof -memprofile=/tmp/fa-mem-scopekey.prof -benchmem -benchtime=2s -count=1
go test ./internal/apply -run '^$' -bench '^BenchmarkCollectStatements_500Transactional$' \
  -memprofile=/tmp/fa-mem-collect.prof -benchmem -benchtime=400ms -count=1

go tool pprof -top -nodecount=25 /tmp/fs-bench.test /tmp/fa-cpu-scanhint.prof | tee "$DOC/cpu-top-ScanWithHint.txt"
go tool pprof -top -alloc_objects -nodecount=25 /tmp/fs-bench.test /tmp/fa-mem-scanhint.prof | tee "$DOC/mem-allocobj-ScanWithHint.txt"
go tool pprof -top -alloc_objects -nodecount=30 /tmp/fs-bench.test /tmp/fa-mem-layout.prof | tee "$DOC/mem-allocobj-LayoutRebuild.txt"
go tool pprof -top -alloc_objects -nodecount=25 /tmp/apply.test /tmp/fa-mem-collect.prof | tee "$DOC/mem-allocobj-CollectStatements.txt"
go tool pprof -top -nodecount=20 /tmp/db-bench.test /tmp/fa-cpu-scopekey.prof | tee "$DOC/cpu-top-ScopeKey.txt"
go tool pprof -top -alloc_objects -nodecount=20 /tmp/db-bench.test /tmp/fa-mem-scopekey.prof | tee "$DOC/mem-allocobj-ScopeKey.txt"
```

## Manual fixes after `-fix`

- **`internal/db/inspector_impl.go`:** `scopePart` field order changed; unkeyed literals `scopePart{'s', name}` became invalid. Replaced with **keyed** literals: `scopePart{s: …, kind: …}`.
- **`internal/engine/engine_test.go`:** anonymous test-case struct field order changed; positional literals broke. Replaced with **keyed** literals `{name: …, run: …}`.

Rule of thumb: after `fieldalignment -fix`, search for **composite literals without field names** on reordered structs and add keys.

## Checked-in artifacts (this directory)

| File | Contents |
| --- | --- |
| `bench-*.txt` | Raw `go test -bench` one-liners for the profiled benchmarks |
| `cpu-top-ScanWithHint.txt` | CPU top — `internal/fs` warm byte-cache + Scan hint |
| `mem-allocobj-*.txt` | `alloc_objects` top for fs / apply / db |

## How it was validated

- **`go test ./...`** — green after the two manual fixes above.
- **Profiles:** confirm stacks symbolize when the `*.test` binary is passed as the first `pprof` argument (see `docs/perf/runs/2026-05-18-scanner-preload-gitinfo-profile/README.md` failure mode if you see only `main.main`).

## What it does not cover

- `benchstat` before/after fieldalignment (not captured here; machine noise dominates small `ns/op` deltas).
- Full-repo `fieldalignment` on every future commit — re-run when large structs change.

## References

- Tool: `fieldalignment` (`golang.org/x/tools/go/analysis/passes/fieldalignment`).
- Prior cold-object narrative: `docs/perf/runs/2026-05-18-cold-object-profile/README.md`.
