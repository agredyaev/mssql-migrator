# Technical Document: MSSQL Integration Test Plan

Lifecycle: `Current`.

## Purpose

This document defines the required live SQL Server checks for the current `rmig` build.

## Scope

- Disposable SQL Server database only
- `plan`, `migrate`, `validate`, `baseline`, and `repair-checksum`
- Metadata objects `[__migrator].migration_runs`, `[__migrator].tracked_schemas`, `[__migrator].tracked_objects`, `[__migrator].schema_migrations`, and `[__migrator].v_migration_state`
- Metadata version object `[__migrator].schema_version`
- Report files in `reports/migration-plan.*`, `reports/migration-report.*`, and `reports/validation-report.*`

## System Context

These checks prove the tool works against a real database, not just unit tests.
They are the gap between local compilation and production readiness.
See `README.md` for the CLI wrapper contract.

## Interfaces And Boundaries

- Inputs: SQL files under `<RM_SQL_ROOT>/<RM_SQL_BASE>`, target database credentials, `RM_*` environment variables
- Outputs: report files in `./reports`, metadata rows in `[__migrator]`
- Ownership boundaries: the test database may be destroyed between runs

## Assumptions And Constraints

- The database is disposable.
- The run is isolated from production data.
- Use `--env pred` for this suite so plan and report artifacts are marked as pre-production runs.
- Repo-driven `migrate` execution is expected to create missing schemas, apply the approved repo object set, record `adopt_existing` metadata rows, and validate the managed object scope.
- Repo-driven `baseline` is expected to create missing repo-managed schemas and objects, adopt already existing objects, and fail closed on checksum drift or missing DDL permission.
- Existing module updates are expected to run only when the repo SQL starts with the matching `CREATE OR ALTER` statement for that object kind.
- Repo-driven `repair-checksum` is expected to resolve one object by repo path or normalized key only when the current plan shows tracked checksum drift for that object, then append a `repair_checksum` attempt row.
- This build persists run state in `[__migrator].migration_runs`, `[__migrator].tracked_schemas`, `[__migrator].tracked_objects`, `[__migrator].schema_migrations`, and `[__migrator].v_migration_state`.
- This build is expected to keep `[__migrator].schema_version` at the current checked-in metadata schema version.

## Nominal Flow

1. Create a repo-driven layout under `<RM_SQL_ROOT>/<RM_SQL_BASE>/<schema>/<kind>/*.sql`.
2. Run `plan` and confirm the plan artifact contains `schema_version`, `sql_root`, `base`, `effective_base_path`, `layout_hash`, `target`, `summary`, `schemas`, `objects`, and `failures`.
3. Run `plan --json` and confirm JSON is written to stdout while logs stay on stderr.
4. Introduce an invalid layout path and confirm planning fails before database changes.
5. Introduce repo-discovered check scripts under `<schema>/checks/*.sql` and confirm `validate` executes them.
6. Create a broken module object and confirm validation fails.
7. Generate an approved plan, change the repo layout, and confirm `migrate` rejects the artifact.
8. Verify `migrate` creates missing schemas, applies only the approved repo object set, skips `adopt_existing` objects without DDL, and records adoption in metadata.
9. Run `baseline` against an empty or partial disposable database and confirm missing repo-managed schemas and objects are created, existing objects are adopted, and run state is written into `[__migrator]`.
10. Remove required DDL permission for a controlled test principal and confirm `baseline` fails with a permission-specific error before or during create work.
11. Repair a stored checksum for an already applied repo-managed object and confirm a new `repair_checksum` attempt row is written.
12. Try `repair-checksum` for an unchanged or untracked object and confirm the command fails closed before metadata mutation.
13. Verify `[__migrator].schema_version` contains the expected current version row.
14. Interrupt a report-writing step in a controlled run and confirm the final report path is never left as a partial file.
15. Run `plan` for a changed existing module without `CREATE OR ALTER` and confirm the plan is blocked before `migrate`.

## Off-Nominal Behavior And Failure Containment

- If repository layout validation fails, the command must stop before object work.
- If validation fails, the report must show the failing object or check script.
- If plan drift appears, the migration must stop.
- If repo-driven migrate execution is reached, the current build must apply only the approved repo object set and run managed-scope post-migrate validation unless skipped.

## Verification And Validation

- `rmig plan --env pred --sql-root ./sql --sql-base dwh`
- `rmig plan --env pred --sql-root ./sql --sql-base dwh --json`
- `rmig migrate --env pred --sql-root ./sql --sql-base dwh --plan-file reports/migration-plan.json`
- `rmig validate --env pred --sql-root ./sql --sql-base dwh`
- `rmig baseline --env pred --sql-root ./sql --sql-base dwh --confirm`
- `rmig repair-checksum --env pred --sql-root ./sql --sql-base dwh --script reporting/views/monthly.sql --confirm`
- Verify `reports/migration-plan.json`, `reports/migration-report.json`, and `reports/validation-report.json` after each corresponding step.
- Retain the generated report files with the test run record until the suite is rerun or replaced.

## Operations And Recovery

- Recreate the disposable database between test runs.
- Re-run the full checklist after any change to parsing, planning, metadata, or SQL execution.

## Open Issues And Non-Goals

- Open issues: this plan does not include a checked-in SQL Server container definition.
- Non-goals: this plan does not replace the external production pipeline.

## References

- `README.md`
- `docs/runbook.md`
- `docs/solution.md`
- `docs/operational-contract.md`
