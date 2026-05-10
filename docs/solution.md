# Technical Document: Solution

Lifecycle: `Current`.

## Purpose

This document describes the current `rmig` solution on the repo-driven v8 runtime path.
It exists so a maintainer can understand what is implemented now, how run state is persisted, and what verification boundaries still remain.

## Scope

- Runtime dispatch: `internal/app/app.go`
- CLI command handlers: `internal/migrator/handler.go`, `internal/migrator/runner.go`, `internal/migrator/baseline_repair.go`, `internal/migrator/validation.go`
- Repo layout discovery: `internal/parser/layout.go`
- Metadata storage: `internal/metadata/metadata.go`
- Shared catalog reads: `internal/catalog/catalog.go`
- Planning logic: `internal/planner/planner.go`
- Report writers: `internal/reports/write.go`
- Validation logic: `internal/validate/validate.go`

## System Context

The solution is a Go CLI named `rmig`.
It is run in a branch-to-PR-to-main workflow, then in an external production job against SQL Server.
See `README.md` for the CLI wrapper contract.
Schema and object scope come from `<RM_SQL_ROOT>/<RM_SQL_BASE>`.

## Interfaces And Boundaries

- Inputs: SQL files under `<RM_SQL_ROOT>/<RM_SQL_BASE>`, `RM_*` environment variables, optional env files loaded through `--env-file` or `RM_ENV_FILE`, command flags, approved plan files
- Outputs: `reports/migration-plan.json`, `reports/migration-plan.txt`, `reports/migration-report.json`, `reports/validation-report.json`, metadata rows in `[__migrator]`, exit codes, logs
- Ownership boundaries: SQL files are owned in Git; `rmig` owns planning, execution, validation, metadata writes, and report generation

## Assumptions And Constraints

- SQL Server is the execution target.
- SQL Server authentication is selected with `RM_DB_AUTH`. `sql` uses explicit login credentials. `integrated` uses Windows Integrated Security through the MSSQL driver.
- Optional dotenv loading is available through `--env-file` or `RM_ENV_FILE`. It does not run by default and does not replace process environment or CLI flag precedence.
- The env file is trusted operator input, but `rmig` accepts only the supported `RM_*` keys that map to current command inputs. Unknown keys fail before command execution.
- `--env` and `RM_ENV` accept only `pred` and `prod`.
- `RM_SQL_ROOT` and `RM_SQL_BASE` are required for planning, execution, validation, and repair commands.
- `RM_SQL_BASE` must be a single directory name under `RM_SQL_ROOT`.
- `plan`, `migrate`, `baseline`, and `repair-checksum` require `RM_GIT_COMMIT`.
- `migrate` requires an approved plan file.
- `plan --json` writes stable machine-readable JSON to stdout and keeps logs on stderr.
- `RM_UPDATE_POLICY` defaults to `none`.
- Existing module updates are allowed only when the repo SQL starts with the matching `CREATE OR ALTER` statement for that object kind.
- `RM_TRANSACTION_MODE` defaults to `script`.
- Logs, reports, and stored error text must not expose secrets.
- Post-SQL metadata writes use `internal/migrator/metadata_context.go` and a short timeout so metadata paths fail quickly instead of waiting for the full command timeout.
- `internal/reports/write.go` writes report artifacts as consistent JSON and text pairs through temporary files and rename publication.
- Repo-driven `migrate` creates missing schemas, applies approved create paths and safe existing-module update paths after plan verification, adopts existing objects without DDL by default, records attempts in `[__migrator]`, and validates the managed object scope by default unless skipped.
- Repo-driven `validate` refreshes module objects, checks existence for the full managed object scope, creates one validation run row, updates tracked object results for successful validation scope, and writes attempt rows only for validation failures and failed checks.
- Repo-driven `baseline` uses the same discovered schema and object scope as `plan` and `migrate`, creates missing schemas and objects, adopts already existing objects, and blocks when a tracked object already exists with checksum drift.
- Repo-driven `baseline` preflights metadata DDL, schema creation permission, object DDL permission, and parent-object availability before create work.
- Repo-driven `repair-checksum` resolves one object by repo path or normalized key, but only when the current plan shows tracked checksum drift for that object. It appends a new successful metadata attempt row in `[__migrator].attempts` instead of mutating old rows in place.
- The append-only metadata history is stored in `[__migrator].attempts`.
- `reports/migration-plan.txt` explains why each planned object is being created, adopted, skipped, updated, or blocked.
- Metadata bootstrap records runtime schema state in `[__migrator].schema_version`, validates known schema versions before upgrade DDL, and avoids recurring DDL churn on current metadata.

## Nominal Flow

1. Load `RM_*` environment variables and command flags in `internal/app/app.go`.
2. Discover schemas, objects, and checks from `<RM_SQL_ROOT>/<RM_SQL_BASE>` with `internal/parser/layout.go`.
3. Run `rmig plan --env prod --sql-root ./sql --sql-base dwh` to produce `reports/migration-plan.json` and `reports/migration-plan.txt`.
4. Run `rmig plan --env prod --sql-root ./sql --sql-base dwh --json` when a machine-readable plan artifact is needed on stdout.
5. Run `rmig migrate --env prod --sql-root ./sql --sql-base dwh --plan-file reports/migration-plan.json` to apply the approved repo-driven object set and validate the managed object scope.
6. Run `rmig validate --env prod --sql-root ./sql --sql-base dwh` to bootstrap metadata if needed, refresh repo-discovered modules, check the full managed object scope, and execute repo-discovered checks.
7. Run `rmig baseline --env prod --sql-root ./sql --sql-base dwh --confirm` when an existing database should be brought under repo-driven metadata control without an approved plan artifact.
8. Run `rmig repair-checksum --env prod --sql-root ./sql --sql-base dwh --script reporting/views/monthly.sql --confirm` only when checksum metadata needs controlled repair for one repo-managed object.

## Off-Nominal Behavior And Failure Containment

- Invalid root or base selection: config validation fails before command execution.
- Invalid repository layout: discovery fails before database work.
- Approved-plan drift: `migrate` fails closed if `git_commit`, `layout_hash`, target, tool identity, comparison mode, update policy, transaction mode, rollback scope, base selection, or the approved schema/object set differs.
- Unsafe existing-module update SQL: `plan` blocks the object when the repo file does not start with the required `CREATE OR ALTER` statement.
- Metadata failure after SQL success: treated as a critical state in the active repo-driven `migrate`, `baseline`, and `repair-checksum` paths.
- Metadata updates fail closed when the target row is missing or duplicated.
- Missing schema creation permission, missing object DDL permission, or missing parent object: create paths fail closed with a specific classified error.
- Scope persistence for repo-managed schemas and objects is written into `[__migrator].items` in one metadata transaction per run scope.
- Validation failure: the run stops and writes `reports/validation-report.*`.
- Repo-driven migrate execution: only the approved repo-driven schema/object set is executed. Repo-discovered checks are outside the `migrate` approval boundary and run only through standalone `validate`.

## Verification And Validation

- See `README.md` for shared build and unit-test commands.
- `go test -race ./...`
- `RMIG_RUN_SQLSERVER_INTEGRATION=1 go test ./internal/migrator -run SQLServer`
- `docs/integration-test-plan.md`

## Operations And Recovery

- Use `docs/runbook.md` after a failed planning, migration, validation, baseline, or repair run.
- Use `rmig baseline` when the repo layout is already the desired target state and the database must be created or adopted into current repo-driven metadata.
- Use `rmig repair-checksum` only when one repo-managed object is already tracked and the current plan shows checksum drift for that object.

## Open Issues And Non-Goals

- Open issues: live SQL Server integration validation is now codified as opt-in tests and still depends on an external disposable SQL Server.
- Non-goals: this document does not define the outer CI/CD pipeline or SQL Server provisioning.

## References

- `README.md`
- `docs/operational-contract.md`
- `docs/runbook.md`
- `docs/integration-test-plan.md`
