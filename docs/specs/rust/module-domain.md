# Technical Document: Module `domain`

Lifecycle: `Current`.

## Purpose

Describe the **in-memory layout model**: object keys, workspace entries, string interning, and catalog/checksum application helpers.

## Scope

- `crates/core/src/domain/workspace.rs` - `Workspace`, object list, digest hooks
- `crates/core/src/domain/object.rs`, `key.rs`, `action.rs`, `kind_code.rs`
- `crates/core/src/domain/schema.rs`, `script.rs`, `store.rs`
- `crates/core/src/domain/arena.rs`, `shared.rs` - string dedup / `SharedStr`
- `crates/core/src/domain/fingerprint.rs` - `key_fingerprint` for normalized path bytes
- `crates/core/src/domain/path_cache.rs` - object/transition path caches on `Workspace`
- `crates/core/src/domain/mod.rs` - `intern_catalog_state`, `for_each_entry`

## System context

`scan` fills `Workspace`; `plan` and `db` read object keys and checksums; `apply` executes per `ObjectEntry`.

## Interfaces and boundaries

- Core type: `Workspace`, `ObjectKey`, `Action`, `key_fingerprint`
- No SQL or filesystem I/O inside `domain`
- Must not import `db`, `driver`, `apply`, `engine`, or other upper layers

## Assumptions and constraints

- Normalized keys: `{schema}/{kind}/{object}` lowercase segments.
- Arena interning reduces allocations on hot plan paths.

## Nominal flow

1. `scan` inserts `ObjectEntry` values into `Workspace`.
2. `plan` reads keys/checksums; `apply` mutates via planned actions only.

## Off-nominal behavior and failure containment

- Failure mode: duplicate normalized keys in layout.
  Containment: scan/plan surface error before apply.

## Operations and recovery

- No runtime operator action; pure in-memory structures.

## Open issues and non-goals

- Non-goals: domain module performs no I/O.

## Verification and validation

- `crates/core/src/domain/arena.rs` tests
- `crates/core/src/domain/key.rs` tests

## References

- `docs/data-oriented-layout-policy.md`
- `docs/specs/rust/module-scan.md`
