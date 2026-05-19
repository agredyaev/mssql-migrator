# Technical Document: Module `internal/fs`

Lifecycle: `Current`.

## Purpose

Describe **repository discovery**: directory walk, `Layout` construction, per-file caching, checksum and optional Git metadata preload, transition ordering, and helpers such as `NormalizeAndHash` / `LayoutHash`.

## Scope

- `internal/fs/scanner.go` — `Scanner`, `Scan`, `preloadChecksums`, `preloadGitInfo`, transition parsing
- `internal/fs/layout.go` — `Layout`, `Object`, `CachedFile` (heap via `Object.File`), path indexes, `LayoutHash`
- `internal/fs/arena.go` — string interning for layout metadata (phase 4)
- `internal/fs/store.go` — `ObjectStore` SoA index (`objectRow`, key map)
- `internal/fs/normalize.go` — SQL normalization and hashing helpers
- Tests and benchmarks: `internal/fs/*_test.go`, `internal/fs/*_bench_test.go`

## System context

`engine` calls `Scanner.Scan(ctx, root)` where `root` derives from `cfg.SQLRoot` / layout roots. The returned `fs.Layout` is the authoritative structure for `db.Inspector` scope and `diff.Computer` inputs.

## Interfaces and boundaries

- Inputs: filesystem paths under `SQLRoot`, optional `GitInfo` / `GitLog` injectable hooks on `Scanner` (tests and benchmarks)
- Outputs: `Layout` value (objects, schemas, transitions, checks); errors on invalid layout or I/O
- Consumers: `internal/engine`, `internal/diff`, `internal/db`, `internal/apply`

## Assumptions and constraints

- Assumption: object paths follow repository conventions validated during scan.
- Constraint: `CachedFile` uses `sync.Once` for lazy content, checksum, and Git fields; concurrent first access must be safe (see implementation).
- Constraint: `LayoutHash` depends on digest collection and sort order; callers use it for plan identity.

## Nominal flow

1. `Scan` walks configured tree, populates `Layout` slices and maps.
2. Optional preload phases reduce hot-path work in `diff.Compute` (checksums, Git metadata).
3. `RebuildPathIndexes` refreshes path maps, interns duplicate strings, and builds `ObjectStore`.

## Off-nominal behavior and failure containment

- Missing files, invalid SQL layout, or Git subprocess failures surface as scan errors; engine fails the command.

## Verification and validation

- `make check` (`internal/fs/fs_test.go` and related tests)

## Operations and recovery

- When changing scan semantics: update `internal/engine` expectations and golden tests if any.

## Open issues and non-goals

- Non-goals: `fs` does not execute SQL or talk to the driver directly.

## References

- `internal/engine/engine.go`
- `internal/diff/diff.go`
- `docs/specs/internals/module-diff.md`
