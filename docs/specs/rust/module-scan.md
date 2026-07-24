# Module `scan`

Lifecycle: `Current`.

## Purpose

Describe **filesystem layout scan**: walk SQL tree, parse object scripts, compute layout digest, optional git preload metadata.

## Scope

- `crates/core/src/scan/walk.rs` - directory walk, object discovery
- `crates/core/src/scan/parse.rs`, `parse_object.rs` - script parsing
- `crates/core/src/scan/digest.rs` - layout digest for cache keys
- `crates/core/src/scan/transition.rs` - transition migration paths
- `crates/core/src/scan/git_preload.rs`, `git_log.rs`, `git_repo.rs` - optional git string preload
- Public entry: `scan::populate`

## System context

First phase of `engine::run_command`. Populates `domain::Workspace`
(`object_entries`, schemas, checksums from file content). Sets
`ws.layout_digest` used by the SQL catalog cache.

## Interfaces and boundaries

- Input: SQL root path, `skip_git` flag
- Output: `scan_ms` timing; mutates `Workspace`
- Downstream: `plan`, `gate`, `db` consume workspace keys and digests
- Path boundary: `walk::scan_root` builds relative wire paths from `Path::components()` and joins components with `/`. It does not rewrite backslash bytes inside a Unix path component into separators.

## Assumptions and constraints

- Layout follows repo SQL tree conventions (see `docs/solution.md`).
- Catalog wire paths (`objectPath`, `transitionPaths` in plan JSON) are **`{database}/{schema}/...`** relative to `RM_SQL_ROOT`. `plan::diff_object` and `plan::transitions` normalize via `domain::with_database_prefix`.
- Git preload is best-effort when `.git` exists at repo root.
- Symlinked files and directories are skipped during directory walk.
- Every path component must be UTF-8 and contain no separators or control characters.
- Each SQL script is limited to `MAX_SQL_SCRIPT_BYTES` (4 MiB).

## Nominal flow

1. `walk::scan_root` - enumerate objects.
2. Parse scripts → `ObjectEntry` list.
3. If not `skip_git`: git preload for author/date metadata.
4. `layout_digest(ws)` stored on workspace.

## Off-nominal behavior and failure containment

- Parse errors fail scan before any SQL I/O.
- Invalid, non-UTF-8, control-character, or oversized inputs fail scan before any SQL I/O.

## Verification and validation

- `crates/core/tests/workflow_integration.rs` (implicit scan on each command)
- `crates/core/tests/scan_walk_test.rs` covers symlink skipping, path validation, valid Unicode, and the SQL size limit.
- `make check`

## Operations and recovery

- After changing layout conventions, update `.temp/sql` fixture and workflow tests.

## Open issues and non-goals

- Non-goals: scan does not run `layout checks` SQL on validate.

## References

- `docs/specs/rust/module-domain.md`
