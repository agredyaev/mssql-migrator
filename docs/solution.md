# Technical Document: Solution

Lifecycle: `Current`.

## Purpose

This document describes the chosen `rmig` solution for MSSQL reporting-layer migrations.
It exists so a maintainer can understand the design choice without reading chat history or a deleted planning note.

## Scope

- Runtime dispatch: `internal/app/app.go`
- CLI command handlers: `internal/migrator/handler.go`, `internal/migrator/runner.go`, `internal/migrator/baseline_repair.go`, `internal/migrator/validation.go`
- Metadata storage: `internal/metadata/metadata.go`
- Planning logic: `internal/planner/planner.go`
- Report writers: `internal/reports/write.go`
- Validation logic: `internal/validate/validate.go`

## System Context

The solution is a Go CLI named `rmig`.
It is run in a branch-to-PR-to-main workflow, then in an external production job against SQL Server.
See `README.md` for the CLI wrapper contract.
The selected design keeps migration history in `__migrator.schema_migrations` and writes reports to `./reports`.

## Interfaces And Boundaries

- Inputs: SQL files in `./sql/versioned`, `./sql/repeatable`, and `./sql/checks`, `RM_*` environment variables, command flags, approved plan files
- Inputs: SQL files in `./sql/versioned`, `./sql/repeatable`, and `./sql/checks`, `RM_*` environment variables, optional env files loaded through `--env-file` or `RM_ENV_FILE`, command flags, approved plan files
- Outputs: `reports/migration-plan.json`, `reports/migration-report.json`, `reports/validation-report.json`, metadata rows, exit codes, logs
- Ownership boundaries: SQL files are owned in Git; `rmig` owns execution, metadata writes, and report generation

## Assumptions And Constraints

- SQL Server is the execution target.
- SQL Server authentication is selected with `RM_DB_AUTH`. `sql` uses explicit login credentials. `integrated` uses Windows Integrated Security through the MSSQL driver.
- Optional dotenv loading is available through `--env-file` or `RM_ENV_FILE`. It does not run by default and does not replace process environment or CLI flag precedence.
- `--env` and `RM_ENV` accept only `pred` and `prod`.
- Versioned scripts are one-time changes.
- Repeatable scripts are tied to Git and rerun only when their checksum changes.
- `plan`, `migrate`, `baseline`, and `repair-checksum` require `RM_GIT_COMMIT`.
- `migrate` requires an approved plan file and runs validation by default; `--skip-validate` or `RM_SKIP_VALIDATE` disables the step.
- Logs, reports, and stored error text must not expose secrets.
- Dangerous repair commands require `--confirm`.

## Nominal Flow

1. Load `RM_*` environment variables and command flags in `internal/app/app.go`.
2. Run `rmig plan --env prod` to produce `reports/migration-plan.json` and `reports/migration-plan.txt`.
3. Run `rmig migrate --env prod --plan-file reports/migration-plan.json` to verify the approved plan, apply SQL, and store history.
4. Run `rmig validate --env prod` to refresh managed objects and execute check scripts from `./sql/checks`.
5. Run `rmig baseline` or `rmig repair-checksum` only as controlled repair actions.

## Off-Nominal Behavior And Failure Containment

- Checksum mismatch: the plan is blocked and migration stops.
- Approved-plan drift: `migrate` fails closed if `git_commit`, `sql_dir_hash`, `target_env`, `target_database`, `tool_version`, `tool_commit`, or the approved script set differs.
- SQL execution failure: the failed attempt is recorded and the run exits with a SQL error code.
- Metadata failure after SQL success: the run fails closed and surfaces a critical state error.
- Validation failure: the run stops and writes `reports/validation-report.*`.

## Verification And Validation

- See `README.md` for shared build and unit-test commands.
- `docs/integration-test-plan.md`

## Operations And Recovery

- Use `docs/runbook.md` after a failed migration or validation run.
- Use `rmig baseline` only for an existing database that needs a starting point.
- Use `rmig repair-checksum` only for already applied scripts with bad stored checksums.

## Open Issues And Non-Goals

- Open issues: the live SQL Server integration suite is still the final proof step.
- Non-goals: this document does not define the outer CI/CD pipeline or SQL Server provisioning.

## References

- `README.md`
- `docs/operational-contract.md`
- `docs/runbook.md`
- `docs/integration-test-plan.md`
