# Technical Document: MSSQL Integration Test Plan

Lifecycle: `Current`.

## Purpose

This document defines the required live SQL Server checks for `rmig`.

## Scope

- Disposable SQL Server database only
- `plan`, `migrate`, `validate`, `baseline`, and `repair-checksum`
- Metadata table `__migrator.schema_migrations`

## System Context

These checks prove the tool works against a real database, not just unit tests.
They are the gap between local compilation and production readiness.

## Interfaces And Boundaries

- Inputs: SQL files in `./sql`, target database credentials, `RM_*` environment variables
- Outputs: report files in `./reports`, metadata rows in `__migrator.schema_migrations`
- Ownership boundaries: the test database may be destroyed between runs

## Assumptions And Constraints

- The database is disposable.
- The run is isolated from production data.
- Script names follow the `V###__name.sql`, `R###__name.sql`, and `C###__name.sql` patterns used by the parser.

## Nominal Flow

1. Bootstrap `__migrator.schema_migrations`.
2. Apply `V001`.
3. Re-run and confirm `V001` is skipped.
4. Modify applied `V001` and confirm checksum mismatch.
5. Apply `R001`.
6. Modify `R001` and confirm repeatable rerun.
7. Create a broken view and confirm validation fails.
8. Add a failing check script and confirm validation fails.
9. Start two migrations and confirm the app lock blocks the second.
10. Change `sql_dir_hash` after planning and confirm migration is blocked.
11. Baseline historical `V` scripts up to a supplied version and confirm history rows are written.
12. Repair a stored checksum for an already applied script and confirm the metadata row updates.

## Off-Nominal Behavior And Failure Containment

- If checksum mismatch appears, the migration must stop.
- If validation fails, the report must show the failing object or check script.
- If the second migration starts, the app lock must block it.

## Verification And Validation

- `rmig plan --env prod`
- `rmig migrate --env prod --plan-file reports/migration-plan.json`
- `rmig validate --env prod`
- `rmig baseline --env prod --up-to V010 --confirm`
- `rmig repair-checksum --env prod --script R002__views.sql --confirm`

## Operations And Recovery

- Recreate the disposable database between test runs.
- Re-run the full checklist after any change to parsing, planning, metadata, or SQL execution.

## Open Issues And Non-Goals

- Open issues: this plan does not include a checked-in SQL Server container definition.
- Non-goals: this plan does not replace the external production pipeline.

## References

- `README.md`
- `docs/runbook.md`
- `docs/implementation-plan.md`
- `docs/deployment-readiness.md`
