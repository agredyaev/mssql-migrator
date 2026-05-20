# Technical Document: Module `engine`

Lifecycle: `Current`.

## Purpose

Describe the **Rust orchestration engine**: connect (or session proxy), scan SQL layout, run plan DB phase, compute diff, optional apply under lock, and emit phase timings.

## Scope

- `rust/crates/core/src/engine/run.rs` — `run_command`, `Command`, `RunOutput`
- `rust/crates/core/src/engine/apply_run.rs` — migrate / baseline apply dispatch, post-apply catalog snapshot
- `rust/crates/core/src/engine/blocked.rs` — blocked migrate scaffold path
- `rust/crates/core/src/engine/filter.rs` — filter already-applied migrations
- `rust/crates/core/src/engine/warm_store.rs` — in-process plan DB snapshot for warm paths
- `rust/crates/core/src/engine/io.rs` — stdout JSON helpers

## System context

`run_command` is the single entry used by `rust/crates/cli`. Flow: connect → `scan::populate` → `db::run_plan_db_phase` → `plan::compute_diff` → command-specific apply (`apply_run::maybe_apply`).

## Interfaces and boundaries

- Public API: `run_command`, `Command`, `RunOutput`, `print_timings_json`, `write_plan_stdout`
- Inputs: `Config`, command enum
- Outputs: exit code, `PhaseTimings`, optional `MigrationPlan`
- Downstream: `scan`, `db`, `plan`, `apply`, `lock`, `scaffold`, `cache`

## Assumptions and constraints

- Plan DB wall is recorded in `timings.parallel_wall_ms` from `db::PlanDbResult`.
- Catalog cache persistence after successful apply: `save_workspace_snapshot` when `applied > 0` (`apply_run.rs`).

## Nominal flow

1. Connect (`driver::connect` or `session::connect_daemon`).
2. Scan workspace (`scan::populate`).
3. Plan DB phase (`db::run_plan_db_phase`) — may L1-hit (zero SQL).
4. `plan::compute_diff`.
5. **Migrate:** if `plan.blocked`, scaffold + exit 10; else lock → `apply::execute_plan` → unlock.
6. Set `plan_wall_ms`, `cli_wall_ms`.

## Off-nominal behavior and failure containment

- `Error::PlanBlocked` on migrate: exit 10, no apply.
- Apply failure: SQL error after lock release ordering in `apply::finish`.

## Verification and validation

- `rust/crates/core/tests/workflow_integration.rs`
- `rust/crates/core/tests/integration_plan.rs`
- `make rust-check`

## Operations and recovery

- When adding a command: extend `Command` enum, CLI `main.rs`, and this document.

## Open issues and non-goals

- Non-goals: engine does not embed bus/subscriber model (Go `internal/bus`).

## References

- `docs/specs/rust/module-scan.md`
- `docs/specs/rust/module-db.md`
- `docs/specs/rust/module-plan.md`
- `docs/specs/rust/module-apply.md`
