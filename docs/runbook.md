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
- Report files in `reports/migration-report.*` and `reports/validation-report.*`

## System Context

The runbook applies when `rmig` has already built a report or has partially changed SQL Server state.
It assumes the operator can read `./reports` and can run the CLI against the target database.
See `README.md` for the CLI wrapper contract.

## Interfaces And Boundaries

- Inputs: `reports/migration-plan.json`, `reports/migration-report.json`, `reports/validation-report.json`, SQL Server error text, `--env`, `--sql-root`, `--sql-base`, `--confirm`
- Outputs: repaired metadata rows, rerun plan artifacts, rerun validation results
- Ownership boundaries: SQL fixes belong in Git; metadata repair belongs to the controlled CLI path

## Assumptions And Constraints

- `baseline` and `repair-checksum` require `--confirm`.
- `repair-checksum` is only for already applied repo-managed objects that already have a successful metadata row.
- `migrate` requires an approved plan file.
- `plan`, `migrate`, `baseline`, and `repair-checksum` require `RM_GIT_COMMIT`.
- `plan`, `migrate`, and `validate` require `--sql-root` and `--sql-base` or the matching `RM_*` environment variables.
- Repo-driven `migrate` execution applies the approved repo-driven schema/object set and validates the managed object scope. Repo-discovered `checks/*.sql` run only in standalone `validate`.
- `baseline` and `repair-checksum` both use the repo-driven layout under `<RM_SQL_ROOT>/<RM_SQL_BASE>`.

## Nominal Flow

1. Read the report file in `./reports`.
2. Identify the failing schema, object, check script, or metadata row.
3. Fix the SQL or metadata issue in the correct Git path under `<RM_SQL_ROOT>/<RM_SQL_BASE>`.
4. Re-run the relevant command.

## Off-Nominal Behavior And Failure Containment

- Plan failure: fix the SQL root/base selection or repository layout, then rerun `plan`.
- Migration failure: inspect the migration report to see whether the failure happened during schema creation, object execution, managed-scope post-migrate validation, or metadata recording, then fix forward in Git and rerun `plan`.
- Metadata failure after SQL success in repo-driven `migrate`, `baseline`, or `repair-checksum`: stop deployment and inspect database state before retrying.
- Metadata failure after SQL success should be treated as a split-brain risk between SQL Server state and `[__migrator]`. Do not retry blindly. Inspect `[__migrator].migration_runs`, `[__migrator].tracked_objects`, `[__migrator].tracked_schemas`, and the target object state first.
- Baseline permission failure: grant the required metadata DDL, schema creation, or object DDL permission, then rerun `plan` or `baseline`.
- Missing parent object for an index or trigger: add or restore the parent table or view in repo scope first, then rerun `plan`.
- Validation failure: fix the broken object or check script, then re-run `validate`.
- Plan drift after approval: regenerate `plan` before retrying `migrate`.

## Verification And Validation

- `rmig plan --env prod --sql-root ./sql --sql-base dwh`
- `rmig plan --env prod --sql-root ./sql --sql-base dwh --json`
- `rmig migrate --env prod --sql-root ./sql --sql-base dwh --plan-file reports/migration-plan.json`
- `rmig validate --env prod --sql-root ./sql --sql-base dwh`
- `rmig baseline --env prod --sql-root ./sql --sql-base dwh --confirm`
- `rmig repair-checksum --env prod --sql-root ./sql --sql-base dwh --script reporting/views/monthly.sql --confirm`

## Operations And Recovery

- `baseline`: use when the repo layout already describes the desired schema/object state and the database should be created or adopted into current repo-driven metadata.
- `repair-checksum`: use only when one repo-managed object already matches the repo SQL in the database but its stored successful checksum row must be repaired.
- After either metadata repair path, re-run `plan`.
- If SQL succeeded but metadata repair is unsafe or blocked, stop and escalate with the report files and database state snapshot.
- If a report file is missing or truncated after a local interruption, rerun the command. Report files are written atomically, so a final artifact should be either the old complete file or the new complete file.

## Open Issues And Non-Goals

- Open issues: this runbook does not define external release approvals or SQL Server provisioning.
- Non-goals: this document does not describe feature development or schema design.

## References

- `README.md`
- `docs/solution.md`
- `docs/operational-contract.md`
- `docs/integration-test-plan.md`
