# Technical Document: MSSQL Integration Test Plan

Lifecycle: `Current`.

## Purpose

This document defines the required live SQL Server checks for the current `rmig` build.

## Scope

- Disposable SQL Server database only
- `plan`, `migrate`, `validate`, `baseline`, and `repair-checksum`
- Metadata objects `[__migrator].runs`, `[__migrator].items`, and `[__migrator].attempts`
- Metadata version object `[__migrator].schema_version`
- Optional report files in `migration-plan.*`, `migration-report.*`, and `validation-report.*` when `--report-dir` is used

## System Context

These checks prove the tool works against a real database, not just unit tests.
They are the gap between local compilation and production readiness.
See `README.md` for the CLI wrapper contract.

## Interfaces And Boundaries

- Inputs: SQL files under `<RM_SQL_ROOT>/<RM_SQL_BASE>`, target database credentials, `RM_*` environment variables
- Outputs: plan output on stdout, optional report files under `--report-dir`, metadata rows in `[__migrator]`
- Ownership boundaries: the test database may be destroyed between runs

## Assumptions And Constraints

- The database is disposable.
- The run is isolated from production data.
- Use `--env pred` for this suite so plan output and optional report artifacts are marked as pre-production runs.
- Repo-driven `migrate` execution is expected to create missing schemas, apply the current repo object set, record `adopt_existing` metadata rows, and validate the managed object scope.
- Repo-driven `baseline` is expected to create missing repo-managed schemas and objects, adopt already existing objects, and fail closed on checksum drift or missing DDL permission.
- Existing module updates are expected to run only when the repo SQL starts with the matching `CREATE OR ALTER` statement for that object kind.
- Tracked table drift is expected to require checked-in transition scripts under `<schema>/tables/_migrations/<table>/` before `migrate` can execute the change.
- Repo-driven `repair-checksum` is expected to resolve one object by repo path or normalized key only when the current plan shows tracked checksum drift for that object and the object is not already on the active transition-backed migrate path, then append a `repair_checksum` row in `[__migrator].attempts`.
- This build persists run state in `[__migrator].runs`, `[__migrator].items`, and `[__migrator].attempts`.
- This build is expected to keep `[__migrator].schema_version` at the current checked-in metadata schema version.

## Nominal Flow

1. Create a repo-driven layout under `<RM_SQL_ROOT>/<RM_SQL_BASE>/<schema>/<kind>/*.sql`.
2. Run `plan` without `--report-dir` and confirm the human-readable plan is written to stdout and no report files are created.
3. Run `plan --json` and confirm JSON is written to stdout while logs stay on stderr.
4. Run `plan --report-dir ./reports` and confirm the persisted plan artifact contains `schema_version`, `sql_root`, `base`, `effective_base_path`, `layout_hash`, `target`, `summary`, `schemas`, `objects`, and `failures`.
5. Introduce an invalid layout path and confirm planning fails before database changes.
6. Introduce repo-discovered check scripts under `<schema>/checks/*.sql` and confirm `validate` executes them.
7. Create a broken module object and confirm validation fails.
8. Generate an approved plan with `--report-dir ./reports`, change the repo layout, and confirm `migrate --plan-file ./reports/migration-plan.json` rejects the artifact.
9. Verify `migrate` without `--plan-file` creates missing schemas, applies the current repo object set, skips `adopt_existing` objects without DDL, and records adoption in metadata.
10. Run `baseline` against an empty or partial disposable database and confirm missing repo-managed schemas and objects are created, existing objects are adopted, and run state is written into `[__migrator]`.
11. Remove required DDL permission for a controlled test principal and confirm `baseline` fails with a permission-specific error before or during create work.
12. Repair a stored checksum for an already applied repo-managed object and confirm a new `repair_checksum` attempt row is written.
13. Try `repair-checksum` for an unchanged or untracked object and confirm the command fails closed before metadata mutation.
14. Verify `[__migrator].schema_version` contains the expected current version row.
15. Interrupt a report-writing step in a controlled run with `--report-dir ./reports` and confirm the final report path is never left as a partial file.
16. Run `plan` for a changed existing module without `CREATE OR ALTER` and confirm the plan is blocked before `migrate`.
17. Run `plan` for a tracked table change without a checked-in transition and confirm the plan stays informational, reports `Blocked: true`, and names the required `<schema>/tables/_migrations/<table>/` path.
18. Add a checked-in table transition under `<schema>/tables/_migrations/<table>/001_<commit>_<slug>.sql`, rerun `plan`, and confirm the planned object carries `transition_paths`.
19. Run `migrate` for that tracked table change and confirm the transition SQL executes before the repo table SQL.
20. Confirm `baseline` stops on the same tracked table change and directs the operator to `migrate` instead of executing the transition path.
21. Try `repair-checksum` for the same transition-backed tracked table change and confirm the command rejects it and tells the operator to use `migrate`.

## Off-Nominal Behavior And Failure Containment

- If repository layout validation fails, the command must stop before object work.
- If validation fails, the report must show the failing object or check script.
- If plan drift appears, the migration must stop.
- If repo-driven migrate execution is reached, the current build must apply the current repo object set and run managed-scope post-migrate validation unless skipped. When `--plan-file` is used, the current set must still match the approved artifact.
- If tracked table drift has no checked-in transition, `plan` must remain read-only and informational while `migrate` continues to fail closed.

## Verification And Validation

- `rmig plan --env pred --sql-root ./sql --sql-base dwh`
- `rmig plan --env pred --sql-root ./sql --sql-base dwh --json`
- `rmig plan --env pred --sql-root ./sql --sql-base dwh --report-dir ./reports`
- `rmig migrate --env pred --sql-root ./sql --sql-base dwh`
- `rmig migrate --env pred --sql-root ./sql --sql-base dwh --plan-file ./reports/migration-plan.json`
- `rmig validate --env pred --sql-root ./sql --sql-base dwh`
- `rmig baseline --env pred --sql-root ./sql --sql-base dwh --confirm`
- `rmig repair-checksum --env pred --sql-root ./sql --sql-base dwh --script reporting/views/monthly.sql --confirm`
- `RMIG_RUN_SQLSERVER_INTEGRATION=1 go test ./internal/migrator -run SQLServer`
- The live test entrypoint is `internal/migrator/sqlserver_integration_test.go` and uses the same `RM_DB_*` connection inputs as the CLI.
- Verify persisted `migration-plan.json`, `migration-report.json`, and `validation-report.json` only for runs that set `--report-dir`.
- Retain generated report files only for runs that set `--report-dir` and need an external test record.

## Operations And Recovery

- Recreate the disposable database between test runs.
- Re-run the full checklist after any change to parsing, planning, metadata, or SQL execution.

## Open Issues And Non-Goals

- Open issues: this plan still does not include a checked-in SQL Server container definition.
- Non-goals: this plan does not replace the external production pipeline.

## References

- `README.md`
- `docs/runbook.md`
- `docs/solution.md`
- `docs/operational-contract.md`
