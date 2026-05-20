# Technical Document: Module `scaffold`

Lifecycle: `Current`.

## Purpose

Describe **blocked DDL scaffold generation**: detect column/table transitions, write `_migrations` SQL files, git integration.

## Scope

- `rust/crates/core/src/scaffold/auto.rs`, `ensure.rs`, `content.rs`
- `rust/crates/core/src/scaffold/column_parser.rs`, `dir.rs`, `git.rs`
- Invoked from `engine::blocked` on blocked migrate

## System context

When `plan.blocked` and command is `Migrate`, engine loads table columns (`db::load_table_columns`) and writes transition SQL under `sql/.../_migrations/` before exit 10.

## Interfaces and boundaries

- Inputs: workspace, plan, column metadata, SQL root path
- Outputs: filesystem scaffold files (consumed by operator commit + re-run migrate)

## Assumptions and constraints

- Assumption: blocked plan includes resolvable table column metadata when needed.
- Constraint: scaffold writes under `_migrations/` beneath object SQL path.

## Nominal flow

1. Engine detects `plan.blocked` on migrate.
2. Load columns for affected tables.
3. Write transition SQL files to disk.
4. Exit 10 without apply.

## Off-nominal behavior and failure containment

- Failure mode: filesystem not writable.
  Containment: I/O error returned; DB unchanged.

## Operations and recovery

- Operator commits generated scaffold, then re-runs migrate.

## Open issues and non-goals

- Non-goals: scaffold does not auto-commit git changes.

## Verification and validation

- `rust/crates/core/tests/scaffold_test.rs`
- `workflow_integration.rs` — scaffold file assertions on blocked phase

## References

- `docs/specs/rust/module-plan.md`
- `docs/specs/internals/module-supporting.md` (Go scaffold)
