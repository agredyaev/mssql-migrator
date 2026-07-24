# ADR-0003: Owned workspace metadata and lazy script bodies

Status: Accepted
Date: 2026-07-21
Updated: 2026-07-24

## Context

The plan phase holds every managed object while it compares repository state
with SQL Server. The previous design used a string arena, offsets, dense rows,
sparse side tables, and a boxed `WorkspaceCold`. That representation reduced
some struct sizes but spread one object across many stores and required custom
resolution code.

SQL script bodies are much larger than their metadata. They must not remain in
memory after scan.

## Decision

- `Workspace` owns ordinary vectors and standard-library maps.
- `ObjectEntry` owns its `ObjectKey`, prior checksum, parent, and transitions.
- `ScriptRow`, `SchemaEntry`, `CatalogObject`, and plan JSON objects use owned
  `String` values.
- `ChecksumMap` and workspace indexes use exact `ObjectKey` values. They do not
  use lossy fingerprints.
- `MigrationPlan` owns one `Vec<PlannedObject>`. It has no parallel plan rows or
  materialization phase.
- Scan reads each SQL file, computes SHA-256, and drops the bytes.
- Apply re-reads only selected files and verifies each body against the
  scan-time checksum in `crates/core/src/apply/script_read.rs`.

## Assumptions and constraints

- The plan output lists every managed object, so plan memory remains
  O(catalog size).
- Normalized keys remain `{schema}/{kind}/{object}`.
- SQL bodies remain bounded by `crates/core/src/file_io.rs`.

## Consequences

- Object metadata is larger, but ownership and lookup are explicit.
- Arena lifetime, offset, fingerprint-collision, cache-rebuild, and side-table
  synchronization failure modes are removed.
- Skip-unchanged objects do not read SQL bodies after scan.
- The committed arm64 baseline records `Workspace = 424` bytes and
  `ObjectEntry = 128` bytes. Runtime allocation and wall-time evidence remains
  the release criterion, not struct size alone.

## Verification

- `cargo test --workspace --all-targets --all-features`
- `make bench-footprint`
- `crates/core/tests/workspace_test.rs`
- `crates/core/tests/plan_json_roundtrip_test.rs`
- `crates/core/src/tests/apply_script_read_test.rs`

## Non-goals

- This decision does not make plan cost independent of catalog size.
- This decision does not cache SQL bodies.

## References

- `docs/data-oriented-layout-policy.md`
- `docs/perf-footprint-audit.md`
- ADR-0011
