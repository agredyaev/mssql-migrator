# Technical Document: Runbook

Lifecycle: `Current`.

## Purpose

This document explains how to operate `rmig` after a failed plan, failed validation, failed migration attempt, or metadata repair.

## Scope

- Failure handling for `rmig plan`
- Failure handling for `rmig migrate`
- Failure handling for `rmig validate`
- Operational use of `rmig baseline`
- Operational use of `rmig repair-checksum`
- Optional report files written through `--report-dir` or `RM_REPORT_DIR`

## System Context

The runbook applies when `rmig` has already printed output or has partially changed SQL Server state.
It assumes the operator can read the original stdout and stderr, can read optional persisted report files if `--report-dir` was used, and can run the CLI against the target database.
See `README.md` for the CLI wrapper contract.

## Interfaces And Boundaries

- Inputs: original stdout and stderr, optional `migration-plan.json`, `migration-report.json`, and `validation-report.json` files under the chosen `--report-dir`, SQL Server error text, `--env`, `--sql-root`, `--sql-base`, `--confirm`
- Outputs: repaired metadata rows, rerun plan output, optional rerun validation report files
- Ownership boundaries: SQL fixes belong in Git; metadata repair belongs to the controlled CLI path

## Assumptions And Constraints

- `baseline` and `repair-checksum` require `--confirm`.
- `repair-checksum` is only for already applied repo-managed objects that already have a successful metadata row and are currently in tracked checksum drift. It is not the normal path for transition-backed tracked table changes because those are meant to run through `migrate`. The command appends a new successful metadata attempt row in `[__migrator].attempts`; it does not rewrite old checksum history.
- When the runbook says metadata attempt history, it means the append-only history stored in `[__migrator].attempts`.
- Current builds do not provide a legacy metadata upgrade path. If legacy objects such as `__migrator.migration_runs` or `__migrator.migration_attempts` exist, `plan`, `migrate`, `validate`, `baseline`, and `repair-checksum` fail closed when they reach metadata bootstrap or checksum reads.
- `plan`, `migrate`, `baseline`, and `repair-checksum` require `RM_GIT_COMMIT`.
- `plan`, `migrate`, and `validate` require `--sql-root` and `--sql-base` or the matching `RM_*` environment variables.
- `--report-dir` is optional. Without it, `rmig` does not persist report files to disk.
- Repo-driven `migrate` execution applies the current repo-driven schema/object set and validates the managed object scope. If `--plan-file` is used, the current set must still match that artifact. Repo-discovered `checks/*.sql` run only in standalone `validate`.
- `baseline` and `repair-checksum` both use the repo-driven layout under `<RM_SQL_ROOT>/<RM_SQL_BASE>`.

## Nominal Flow

1. Read the original stdout and stderr, or read the persisted report file in the chosen `--report-dir` when one exists.
2. Identify the failing schema, object, check script, or metadata row.
3. Fix the SQL or metadata issue in the correct Git path under `<RM_SQL_ROOT>/<RM_SQL_BASE>`.
4. Re-run the relevant command.

## Off-Nominal Behavior And Failure Containment

- Plan failure: fix the SQL root/base selection or repository layout, then rerun `plan`.
- Plan metadata failure: `plan` is read-only and does not repair partial metadata. Use `migrate`, `baseline`, or `repair-checksum` when the environment needs metadata bootstrap under lock, then rerun `plan`.
- Plan output includes per-object reasoning. Use stdout by default, or `migration-plan.txt` when `--report-dir` was used, to see why an object is being created, adopted, skipped, updated, or blocked.
- Safe additive tracked table drift: rerun `plan` and confirm it auto-created `<schema>/tables/_migrations/<table>/001_<commit>_auto_add_columns.sql` and now lists that transition path. Then run `migrate` on that transition-backed path.
- Non-safe tracked table drift without transitions: rerun `plan` if needed and inspect the auto-created scaffold under `<schema>/tables/_migrations/<table>/001_<commit>_describe_change.sql`. Replace that scaffold with real checked-in SQL, rename it if needed to the final `<nnn>_<commit>_<slug>.sql` form, rerun `plan`, and confirm the block reason is gone before `migrate`.
- Transition-backed tracked table drift: review the listed transition paths in the plan output, then use `migrate`. Do not use `baseline` for that path.
- Migration failure: inspect the migration report to see whether the failure happened during schema creation, object execution, managed-scope post-migrate validation, or metadata recording, then fix forward in Git and rerun `plan`.
- Metadata failure after SQL success in repo-driven `migrate`, `baseline`, or `repair-checksum`: stop deployment and inspect database state before retrying.
- Metadata failure after SQL success should be treated as a split-brain risk between SQL Server state and `[__migrator]`. Do not retry blindly. Inspect `[__migrator].runs`, `[__migrator].items`, `[__migrator].attempts`, and the target object state first.
- Legacy metadata incompatibility: stop and escalate. The current CLI does not upgrade legacy metadata in place, so do not drop or rewrite old metadata objects during deployment unless you have a separate approved migration procedure.
- Baseline permission failure: grant the required metadata DDL, schema creation, or object DDL permission, then rerun `plan` or `baseline`.
- Missing parent object for an index or trigger: add or restore the parent table or view in repo scope first, then rerun `plan`.
- Validation failure: fix the broken object or check script, then re-run `validate`.
- Plan drift after approval: regenerate `plan` before retrying `migrate`.

## Verification And Validation

- `rmig plan --env prod --sql-root ./sql --sql-base dwh`
- `rmig plan --env prod --sql-root ./sql --sql-base dwh --json`
- `rmig migrate --env prod --sql-root ./sql --sql-base dwh`
- `rmig migrate --env prod --sql-root ./sql --sql-base dwh --plan-file ./reports/migration-plan.json`
- `rmig validate --env prod --sql-root ./sql --sql-base dwh`
- `rmig baseline --env prod --sql-root ./sql --sql-base dwh --confirm`
- `rmig repair-checksum --env prod --sql-root ./sql --sql-base dwh --script reporting/views/monthly.sql --confirm`

## Operations And Recovery

- `baseline`: use when the repo layout already describes the desired schema/object state and the database should be created or adopted into current repo-driven metadata.
- `repair-checksum`: use only when one repo-managed object is already tracked and the current plan shows checksum drift for that object without an intended transition-backed `migrate` path.
- After either metadata repair path, re-run `plan`.
- If SQL succeeded but metadata repair is unsafe or blocked, stop and escalate with the report files and database state snapshot.
- If a report file is missing or truncated after a local interruption and `--report-dir` was used, rerun the command when it is safe to do so. Report JSON and text artifacts are published as a consistent pair, so the final state should be either the old pair or the new pair.

## Open Issues And Non-Goals

- Open issues: this runbook does not define external release approvals or SQL Server provisioning.
- Non-goals: this document does not describe feature development or schema design.

## References

- `README.md`
- `docs/solution.md`
- `docs/operational-contract.md`
- `docs/integration-test-plan.md`
