# Module `engine`

Lifecycle: `Current`.

## Purpose

Describe the **Rust orchestration engine**: connect (or session proxy), scan SQL layout, run plan DB phase, compute diff, optional apply under lock, and emit phase timings.

## Scope

- `crates/core/src/engine/run/mod.rs` - `run_command`, `Command`, `RunOutput`
- `crates/core/src/engine/apply_run.rs` - migrate / baseline apply dispatch, post-apply catalog snapshot
- `crates/core/src/engine/blocked.rs` - blocked migrate scaffold path
- `crates/core/src/engine/filter.rs` - filter already-applied migrations
- `crates/core/src/engine/io.rs` - stdout JSON helpers

## System context

`run_command` is the single entry used by `crates/cli`. Flow: discover catalog databases → (`ensure_catalog_databases_exist` for mutate commands only) → `scan::populate` → `db::run_plan_db_phase` → `plan::compute_diff` → command-specific apply (`apply_run::maybe_apply`).

## Interfaces and boundaries

- Public API: `run_command`, `Command`, `RunOutput`, `print_timings_json`, `write_plan_stdout`
- Inputs: `Config`, command enum
- Outputs: exit code, `PhaseTimings`, optional `MigrationPlan`
- Downstream: `scan`, `db`, `plan`, `apply`, `lock`, `scaffold`, `cache`

## Assumptions and constraints

- Plan DB wall is recorded in `timings.parallel_wall_ms` from `db::PlanDbResult`.
- Advisory lock scope: `apply_run.rs` calls `lock::release_after_body` even when `execute_plan` fails (BG-001).
- Catalog cache persistence after successful apply: `save_workspace_snapshot` when `applied > 0` (`apply_run.rs`).

## Nominal flow

1. Resolve catalog database names; call `ensure_catalog_databases_exist` only for `migrate`, `baseline`, and `repair-checksum`. `plan` and `validate` connect to existing databases and fail if a target database is missing.
2. Scan workspace (`scan::populate`).
3. Per database: connect (`driver::connect` or `session::connect_daemon`) and run `db::run_plan_db_phase`; managed workspaces refresh live-state fingerprints instead of accepting a zero-SQL L1 hit.
4. `plan::compute_diff`.
5. Merge per-database plans when multiple catalog databases are present.
6. **Migrate:** if `plan.blocked`, scaffold + exit 10; else lock → `apply::execute_plan` → unlock.
7. Set `plan_wall_ms`, `cli_wall_ms`.

## Off-nominal behavior and failure containment

- `Error::PlanBlocked` on migrate: exit 10, no apply.
- Apply failure: SQL error after lock release ordering in `apply::finish`.

## Verification and validation

- `crates/core/tests/workflow_integration.rs`
- `crates/core/tests/integration_plan.rs`
- `crates/core/tests/plan_no_db_side_effect_test.rs`
- `make check`

## Operations and recovery

- When adding a command: extend `Command` enum, CLI `main.rs`, and this document.

## Open issues and non-goals

- Non-goals: engine does not embed a pub/sub subscriber model.

## References

- `docs/specs/rust/module-scan.md`
- `docs/specs/rust/module-db.md`
- `docs/specs/rust/module-plan.md`
- `docs/specs/rust/module-apply.md`
