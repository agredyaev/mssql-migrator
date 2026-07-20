# Module `apply`

Lifecycle: `Current`.

## Purpose

Describe **DDL/DML apply execution**: schemas, table transitions, dependent objects, transaction boundaries, audit flush, and cache invalidation.

## Scope

- `crates/core/src/apply/mod.rs` - `execute_plan` orchestration
- `crates/core/src/apply/schemas.rs`, `objects.rs`, `objects_exec.rs`, `transitions.rs`
- `crates/core/src/apply/history_write.rs`, `kind.rs`, `result.rs`

## System context

Called from `engine::apply_run` after `lock::acquire`; `lock::release_after_body` runs even when apply fails. On success with applied objects, invalidates catalog cache and L1; engine may call `save_workspace_snapshot` separately.

## Interfaces and boundaries

- Public: `execute_plan` → `ApplyResult`
- Inputs: `Config`, `TimingConn`, `Workspace`, `MigrationPlan`
- Must not run when `plan.blocked` (returns `Error::PlanBlocked`)

## Assumptions and constraints

- Apply order: schemas → table transitions → indexes and programmable objects. This lets same-run dependents use columns introduced by a transition.
- Failed object stops batch; partial errors returned as `Error::Sql`.
- `baseline` and `repair-checksum` use `execute_metadata_plan`; they never execute repository SQL.

## Nominal flow

1. `ensure_tables`.
2. Apply schema DDL.
3. Apply pending table transition migrations.
4. Apply indexes and programmable object scripts per plan.
5. `flush_history`, invalidate caches on success.

## Verification and validation

- `crates/core/tests/workflow_integration.rs`
- `make workflow-fast`

## Off-nominal behavior and failure containment

- Failure mode: apply SQL error mid-batch.
  Containment: return `Error::Sql`; partial history flush only when records collected.

## Operations and recovery

- Routine operation: DDL execution is invoked only by `migrate`; metadata-only adoption/repair uses the separate executor.

## Open issues and non-goals

- Non-goals: apply does not re-plan after failure.

## References

- `docs/specs/rust/module-engine.md`
- `docs/prod-gate.md`
