# Module `plan`

Lifecycle: `Current`.

## Purpose

Describe **migration plan computation**: apply catalog/checksums to workspace, build inspect scope, diff file vs DB state, classify actions, blocked detection.

## Scope

- `crates/core/src/plan/diff.rs`, `diff_decide.rs`, `diff_object.rs`, `diff_ctx.rs` - diff engine
- `crates/core/src/plan/scope.rs`, `scope_build.rs` - inspect scope, stable vs hot keys
- `crates/core/src/plan/git_scope.rs` - git hot scope JSON for catalog SQL
- `crates/core/src/plan/scenario.rs` - action scenarios (create, update, adopt, blocked)

## System context

After plan DB phase, `compute_diff` compares workspace file checksums and catalog existence against audit history. Output is `export::MigrationPlan` JSON.

## Interfaces and boundaries

- Public: `compute_diff`, `compute_diff_into`, `build_inspect_scope`, `git_hot_scope_json`
- Inputs: `Workspace`, `CatalogState`, `ChecksumMap`
- Outputs: `MigrationPlan`, diff timing ms
- Upstream: `db`, `scan`; downstream: `apply`, `scaffold`, `gate`

## Assumptions and constraints

- Stable objects (file checksum == audit history, outside git delta) merge into catalog without SQL.
- `RMIG_CATALOG_SPOTCHECK` promotes stable keys to hot for SQL verification.

## Nominal flow

1. `apply_catalog_if_needed` / `apply_checksums_if_needed` on workspace.
2. `build_inspect_scope` (full or git delta + checksums).
3. Per-object diff → planned actions.
4. Blocked when DDL change requires transition scaffold not yet committed.

## Off-nominal behavior and failure containment

- Blocked plan: migrate stops at engine layer (exit 10).

## Verification and validation

- `crates/core/tests/plan_diff_test.rs`
- `crates/core/tests/workflow_integration.rs` - blocked + view update actions
- `make prod-gate`

## Operations and recovery

- When changing action taxonomy, update gate snapshot tests and baseline JSON if wire shape changes.

## Open issues and non-goals

- Non-goals: plan module does not execute SQL.

## References

- `docs/specs/rust/module-db.md`
- `docs/specs/rust/module-db.md`
