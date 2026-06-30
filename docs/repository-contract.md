# Repository Contract

Lifecycle: `Current`.

## Purpose

Define the on-disk structure and naming rules that `rmig` reads, validates, and
deploys. It answers: "What layout must a SQL repository follow so the same tree
always produces the same, safe migration plan?"

## Scope

- Filesystem walk and parsing: `crates/core/src/scan/walk.rs`, `crates/core/src/scan/parse_object.rs`, `crates/core/src/scan/transition.rs`.
- Identifier and path validation: `crates/core/src/sql_ident.rs`.
- Key normalization and duplicate detection: `crates/core/src/domain/key.rs`, `crates/core/src/domain/workspace/objects/ingest.rs`, `crates/core/src/domain/workspace/compact.rs`.
- Out of scope: SQL body semantics inside each script (the author owns those).

## System Context

`rmig` is stateless: it matches the declared filesystem layout against SQL
Server's live catalog. The root is `RM_SQL_ROOT` (see `docs/operational-contract.md`).
Each top-level directory under the root is a database name. The tool scans the
tree once, builds a normalized in-memory workspace, and diffs it against the
catalog.

Object ownership has three states. A *managed* object is represented by a `.sql`
file in the tree; it is the only kind any command acts on. An *unmanaged* object
exists in the live catalog but has no file in the tree; the diff never enumerates
it, so it is never created, altered, or dropped. An *orphaned* object was managed
previously but its file has since been removed from the tree; it is treated
exactly like an unmanaged object and preserved. Because the diff iterates only
workspace (repository) objects, an object's absence from the tree can never
produce a destructive operation.

## Interfaces And Boundaries

- Inputs: a directory tree of `.sql` files under `RM_SQL_ROOT`.
- Object path: `<database>/<schema>/<object-type>/<object-name>.sql`.
- Object types (`<object-type>`): `tables`, `views`, `procedures`, `functions`, `triggers`, `indexes`, `types`, `sequences`, `synonyms` (the `KINDS` list in `crates/core/src/scan/parse_object.rs`).
- Transition (table migration) path: `<database>/<schema>/tables/_migrations/<table>/<ordinal>_<commit>_<slug>.sql`.
- Check scripts: any path containing `checks/` is ingested as a non-deploying check.
- Outputs: a deterministic migration plan (see `docs/migration-flow.md`).
- Managed scope: only objects with a `.sql` file under `RM_SQL_ROOT` are managed. Catalog objects without a corresponding file are out of scope for every command and are never created, altered, or dropped.
- Ownership boundary: path/identifier validity is owned by `crates/core/src/sql_ident.rs`; SQL correctness is owned by the script author.

## Assumptions And Constraints

- Assumptions: file paths and contents are untrusted input; database/schema/object
  names come from directory and file names.
- Constraints:
  - Paths must be valid UTF-8; non-UTF-8 paths are rejected.
  - Each path component must be non-empty and must not be `.`, `..`, or contain `/`, `\`, or NUL.
  - Object keys are normalized to lowercase `schema/kind/name`; they are therefore case-insensitive.
  - Generated T-SQL identifiers (schema, database) are limited to 128 characters.
  - Transition filename: `<ordinal>` is exactly 3 digits, `<commit>` is at least 7 hex digits, `<slug>` is non-empty.
  - Symbolic links are skipped during the walk.
  - Safe default: only objects explicitly represented in the repository are managed. Existing database objects not represented in the repository are treated as unmanaged and preserved.
  - Deletion is not supported: removing a file from the tree never drops its database object, and the tool never infers a drop from absence. An intentional drop must be authored explicitly as a table transition script under `_migrations/`.
  - First adoption: `baseline` (and the first `migrate`) record a checksum only for repository objects that already exist in the database; database-only objects are left unmanaged, not adopted.

## Nominal Flow

1. Walk `RM_SQL_ROOT`, collecting `.sql` files in sorted order (`crates/core/src/scan/walk.rs`).
2. Route each file: `_migrations/` paths to transitions, `checks/` paths to checks, all others to object ingest.
3. Validate path components, parse the object key, and store it; keys are sorted, so the plan is deterministic.
4. Hand the normalized workspace to the diff and plan stages (`docs/migration-flow.md`).

## Off-Nominal Behavior And Failure Containment

- Failure mode: two files normalize to the same `schema/kind/name` key (including case-only differences).
  Containment: scan fails closed with `duplicate object ...` (`crates/core/src/domain/workspace/objects/ingest.rs`); no object is silently dropped.
- Failure mode: two transitions for one table share the same ordinal.
  Containment: scan fails closed with `duplicate transition ordinal ...` (`crates/core/src/domain/workspace/compact.rs`). Gaps in ordinals are allowed.
- Failure mode: a file sits under an unrecognized object-type folder at object depth.
  Containment: a `tracing::warn!` is emitted and the file is skipped, not silently ignored (`crates/core/src/scan/parse_object.rs`).
- Failure mode: invalid path component or non-UTF-8 path.
  Containment: scan returns `Error::InvalidInput`; nothing is deployed.
- Failure mode: a database object has no corresponding file in the tree (unmanaged, or orphaned after a file removal).
  Containment: the diff enumerates only workspace objects (`crates/core/src/plan/diff.rs`), so the object is never planned — it is a safe no-op, preserved and never dropped or altered. Deletion requires an explicit transition script.

## Verification And Validation

- Contracts and checks: `crates/core/tests/scan_walk_test.rs` (duplicate ordinal, backslash, symlink), `crates/core/src/tests/sql_ident_test.rs` (path/identifier rules, 128-char limit), `crates/core/src/domain/workspace/objects/ingest.rs` tests (duplicate key).
- Safety guards: `crates/core/tests/unmanaged_objects_test.rs` (unmanaged objects are never planned, partial repositories leave others untouched, removal does not drop) and `crates/core/tests/existing_db_adoption_integration.rs` (real-database preservation across `migrate`).
- Evidence artifacts: test output from `cargo test -p migrator-core --lib --tests`.
- Exit criteria: every invalid layout above produces a clear error; every valid layout produces an identical plan across runs.

## Operations And Recovery

- Routine operation: organize scripts as `<database>/<schema>/<object-type>/<name>.sql`; add table changes as ordinal-numbered transitions.
- Recovery or rollback: fix the offending file (rename a duplicate, renumber an ordinal, move a misplaced file) and re-run; scan is read-only and safe to repeat.

## Open Issues And Non-Goals

- Open issues: object names (from directory/file names) are not length-checked because the tool does not emit them as identifiers; the author's SQL does.
- Non-goals: this document does not define SQL body syntax, deployment ordering across object kinds, or catalog connection settings.

## References

- Canonical source paths: `crates/core/src/scan/walk.rs`, `crates/core/src/scan/parse_object.rs`, `crates/core/src/scan/transition.rs`, `crates/core/src/sql_ident.rs`.
- Related contracts and scripts: `docs/operational-contract.md`, `docs/migration-flow.md`.
- Related runbooks or ADRs: `docs/runbook.md`.
