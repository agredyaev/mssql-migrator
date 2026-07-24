# Module `domain`

Lifecycle: `Current`.

## Purpose

Describe the in-memory repository model used by scan, plan, and apply.

## Scope

- `crates/core/src/domain/workspace/` - `Workspace`, object and script indexes
- `crates/core/src/domain/object/mod.rs` - `ObjectEntry`, parent, transitions
- `crates/core/src/domain/key.rs` - `ObjectKey`, `ScriptKey`
- `crates/core/src/domain/script.rs` - script path, checksum, Git metadata
- `crates/core/src/domain/schema.rs`, `transition.rs`, `layout_path.rs`

## System Context

`scan` fills one `Workspace`. `db` adds live catalog facts and prior checksums.
`plan` builds owned `PlannedObject` values. `apply` re-reads selected scripts
from their stored absolute paths.

## Interfaces And Boundaries

- Inputs: normalized repository paths and scan-time SHA-256 checksums.
- Outputs: `Workspace`, `ObjectEntry`, `ObjectKey`, `ScriptRef`.
- Ownership boundary: `domain` owns metadata only. It performs no SQL or
  filesystem I/O and does not import `db`, `driver`, `apply`, or `engine`.

`Workspace` uses `Vec`, `HashMap`, `String`, and exact `ObjectKey` values.
Object state is stored on `ObjectEntry`; there are no arena offsets or parallel
side tables.

## Assumptions And Constraints

- Object keys are lowercase `{schema}/{kind}/{object}` strings.
- Script and object IDs are 1-based inside a workspace.
- SQL bodies are not retained in the domain model.
- Plan memory is O(number of managed objects).

## Nominal Flow

1. Scan inserts scripts and `ObjectEntry` values.
2. `Workspace::finalize_object_layout` sorts objects, builds exact-key indexes,
   and attaches staged transitions.
3. Database inspection updates `db_exists`, `prior_checksum`, and `parent`.
4. Plan and apply read that single object representation.

## Off-Nominal Behavior And Failure Containment

- Failure mode: two files normalize to the same object key.
  Containment: `Workspace::push_object` returns `Error::InvalidInput` before
  planning.
- Failure mode: a transition has no matching table or script.
  Containment: finalization drops it with a warning; no invalid script ID enters
  an apply plan.

## Verification And Validation

- `cargo test -p migrator-core --all-features --lib --tests`
- `crates/core/tests/workspace_test.rs`
- `crates/core/tests/delta_scope_test.rs`
- `crates/core/tests/scan_walk_test.rs`
- Exit criterion: exact-key lookup, transition relinking, and per-database
  workspace tests pass.

## Operations And Recovery

- Runtime operators do not maintain this state; each command rebuilds it.
- Recovery from invalid layout is to fix the named repository path and rerun.

## Open Issues And Non-Goals

- Open issues: none.
- Non-goals: caching SQL bodies or hiding SQL Server state behind a local
  snapshot.

## References

- `docs/data-oriented-layout-policy.md`
- `docs/specs/rust/module-scan.md`
- `adr/0003-data-oriented-arena-workspace.md`
