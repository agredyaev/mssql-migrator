# Workspace Layout and Footprint Policy

Lifecycle: `Current`.

## Purpose

Define the in-memory layout rules for `migrator-core`. The policy keeps the
plan path understandable while retaining measurable footprint and wall-time
gates.

## Scope

- `crates/core/src/domain/`
- `crates/core/src/plan/`
- `crates/core/src/export/`
- `crates/core-dev/src/perf/`
- `crates/core/tests/testdata/perf/footprint_baseline.json`

## System Context

Each command scans every managed SQL object and emits one plan object. The
current output contract therefore requires O(N) metadata for N repository
objects.

The previous layout used a string arena, byte offsets, fingerprints, dense
rows, and sparse side tables. Those structures reduced selected `size_of`
values but made one logical object depend on several synchronized stores.
Current code uses standard owned values.

## Interfaces And Boundaries

- Inputs: repository paths, scan-time SHA-256 checksums, SQL Server catalog
  rows, and audit checksums.
- Outputs: `Workspace`, `MigrationPlan`, plan JSON, and apply records.
- `Workspace::object_entries` is the canonical object collection.
- `ObjectEntry` owns its key, checksum, prior checksum, parent, and transitions.
- `MigrationPlan::objects` is the canonical plan collection.
- `HashMap<ObjectKey, _>` provides exact-key lookup. Fingerprints are not object
  identity.
- SQL bodies remain outside the model. Apply reads and verifies them on demand.

## Assumptions And Constraints

- Normalized object identity is `{schema}/{kind}/{object}`.
- Plan output contains every managed object.
- SQL script size is bounded by `crates/core/src/file_io.rs`.
- Struct sizes differ by target and compiler. The committed baseline applies
  only to its recorded `target` and `rustc_version`.
- Runtime correctness and measured SLOs take priority over smaller structs.

## Nominal Flow

1. `scan` creates owned `ScriptRow`, `SchemaEntry`, and `ObjectEntry` values.
2. `Workspace::finalize_object_layout` sorts objects and builds exact-key
   indexes.
3. `db` adds live existence, parent, and prior-checksum facts to each object.
4. `plan` creates one owned `PlannedObject` per workspace object.
5. `apply` re-reads only selected SQL files and verifies their checksums.

## Off-Nominal Behavior And Failure Containment

- Failure mode: two paths normalize to the same object key.
  Containment: scan returns `Error::InvalidInput` before database mutation.
- Failure mode: a script changes after scan.
  Containment: `apply/script_read.rs::verified_body` rejects the checksum
  mismatch before execution.
- Failure mode: a layout change increases memory or latency.
  Containment: `footprint_baseline_match`, criterion output, and E2E SLO gates
  expose the change before release.

## Verification And Validation

- `cargo test --workspace --all-targets --all-features`
- `make bench-footprint`
- `make bench-footprint-profile`
- `make bench-footprint-alloc`
- `make plan-db-perf`
- `make slo`
- Evidence:
  `crates/core/tests/testdata/perf/footprint_baseline.json` and generated files
  under `ops/perf/artifacts/`.
- Exit criterion: tests pass, intentional struct-size changes have a refreshed
  baseline, and the configured SLO gates pass.

## Operations And Recovery

- Run `make bench-footprint-update-baseline` only for an intentional type-layout
  change on the recorded target.
- Use `make bench-footprint-profile` or `make bench-footprint-alloc` before
  adding a custom storage layer.
- If runtime evidence regresses, revert the layout change or fix the measured
  path before updating a baseline.

## Open Issues And Non-Goals

- Open issue: the committed baseline covers `aarch64-apple-darwin`; CI remains
  responsible for functional validation on Linux.
- Non-goal: make full-plan memory independent of catalog size.
- Non-goal: add arenas, offset tables, lossy fingerprints, or parallel plan
  rows without profiler evidence and a measured product-scale need.

## References

- `adr/0003-data-oriented-arena-workspace.md`
- `adr/0011-plan-memory-o-catalog.md`
- `docs/perf-footprint-audit.md`
- `docs/specs/rust/module-domain.md`
- `ops/perf/README.md`
