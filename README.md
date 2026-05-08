# rmig

Lifecycle: `Current`.

## Purpose

`rmig` is the Go CLI contract for MSSQL reporting-layer migrations, validation, and metadata capture.
The runtime behavior is implemented in `internal/app/app.go` and the `internal/migrator/` packages.

## Scope

- Runtime dispatch: `internal/app/app.go`
- CLI command handlers: `internal/migrator/handler.go`, `internal/migrator/runner.go`, `internal/migrator/baseline_repair.go`, `internal/migrator/validation.go`
- Metadata store: `internal/metadata/metadata.go`
- Planning logic: `internal/planner/planner.go`
- Reports: `internal/reports/write.go`
- Validation: `internal/validate/validate.go`
- Canonical solution: `docs/solution.md`
- Canonical operational contract: `docs/operational-contract.md`
- Canonical runbook: `docs/runbook.md`
- Canonical integration checks: `docs/integration-test-plan.md`

## CLI Wrapper Contract

`cmd/rmig/main.go` is the CLI entrypoint.
It calls `internal/app.Run(os.Args, buildInfo)` and passes `internal/app.BuildInfo{Version: <version>, Commit: <git sha>}`.
`rmig version` prints `rmig <version> commit=<sha>`.

## System Context

The expected flow is branch work on `main`, a PR to `main`, then a production pipeline that runs `rmig` against SQL Server.
The tool reads SQL files from `./sql/versioned`, `./sql/repeatable`, and `./sql/checks`, writes reports to `./reports`, and records execution history in `__migrator.schema_migrations`.

## Interfaces And Boundaries

- Inputs: `RM_DB_SERVER`, `RM_DB_PORT`, `RM_DB_DATABASE`, `RM_DB_USER`, `RM_DB_PASSWORD`, `RM_MANAGED_SCHEMAS`, `RM_GIT_COMMIT`, `RM_GIT_BRANCH`, `RM_PIPELINE_RUN_ID`, `RM_PIPELINE_URL`, `RM_ACTOR`, `--env`, `--plan-file`, `--up-to`, `--script`, `--confirm`, `--skip-validate`, SQL files in `./sql/versioned`, `./sql/repeatable`, and `./sql/checks`
- Outputs: `reports/migration-plan.json`, `reports/migration-plan.txt`, `reports/migration-report.json`, `reports/migration-report.txt`, `reports/validation-report.json`, `reports/validation-report.txt`, metadata rows in `__migrator.schema_migrations`, exit codes, logs
- Ownership boundaries: SQL Server access is external; `rmig` owns planning, migration, validation, and metadata writes

## Flags And Env

- Common flags: `--env`, `--sql-dir`, `--report-dir`, `--log-level`, `--json-logs`, `--timeout`, `--script-timeout`, `--lock-timeout`
- Command flags: `--plan-file` for `migrate`, `--skip-validate` for `migrate`, `--up-to` and `--confirm` for `baseline`, `--script` and `--confirm` for `repair-checksum`
- Supporting environment: `RM_ENV`, `RM_SQL_DIR`, `RM_REPORT_DIR`, `RM_LOG_LEVEL`, `RM_JSON_LOGS`, `RM_TIMEOUT`, `RM_SCRIPT_TIMEOUT`, `RM_LOCK_TIMEOUT`, `RM_PLAN_FILE`, `RM_SKIP_VALIDATE`, `RM_BASELINE_UP_TO`, `RM_REPAIR_SCRIPT`, `RM_CONFIRM`

## Assumptions And Constraints

- SQL Server is reachable with the configured credentials.
- `baseline` and `repair-checksum` require `--confirm`.
- `plan`, `migrate`, `baseline`, and `repair-checksum` require `RM_GIT_COMMIT`.
- Versioned scripts are applied once.
- Repeatable scripts rerun only when their checksum changes.
- `migrate` requires `--plan-file` and runs post-migration validation by default; `--skip-validate` or `RM_SKIP_VALIDATE` disables that step.
- Logs, reports, and stored error text must not expose secrets.
- Script names follow `V###__name.sql`, `R###__name.sql`, and `C###__name.sql`.

## Nominal Flow

1. Build with `go build -ldflags "-X main.version=0.1.0-dev -X main.commit=$(git rev-parse HEAD)" -o rmig ./cmd/rmig`.
2. Run `rmig plan --env prod` to write `reports/migration-plan.json` and `reports/migration-plan.txt`.
3. Run `rmig migrate --env prod --plan-file reports/migration-plan.json` to verify the approved plan, apply SQL, and write migration reports.
4. Run `rmig validate --env prod` to refresh managed objects and execute check scripts from `./sql/checks`.
5. Use `rmig baseline` or `rmig repair-checksum` only for controlled metadata repair.

## Off-Nominal Behavior And Failure Containment

- Checksum mismatch: `plan` returns a blocked result and `migrate` fails closed.
- Approved-plan drift: `migrate` rejects the plan if `git_commit`, `sql_dir_hash`, `target_env`, `target_database`, `tool_version`, `tool_commit`, or the approved script set differs.
- Metadata write failure: `migrate` reports `critical_state` and stops.
- SQL execution failure: the current script fails, the attempt is recorded, and the run exits with a SQL error code.
- Concurrent migration: app lock blocks the second run.
- Validation failure: the run stops and writes `reports/validation-report.*`.

## Verification And Validation

- `PATH=/usr/local/go/bin:$PATH go test ./...`
- `PATH=/usr/local/go/bin:$PATH go vet ./...`
- `PATH=/usr/local/go/bin:$PATH go build -ldflags "-X main.version=0.1.0-dev -X main.commit=$(git rev-parse HEAD)" -o rmig ./cmd/rmig`
- `./rmig version`
- Unit and contract tests in `internal/app/app_test.go`, `internal/config/config_test.go`, `internal/logger/logger_test.go`, `internal/planner/planner_test.go`, `internal/parser/*_test.go`, `internal/state/state_test.go`, `internal/checksum/checksum_test.go`, `internal/validate/validate_test.go`, `internal/reports/report_schema_test.go`
- Documentation governance: `docs/specs/documentation-spec.md` and `docs/specs/nasa-document-spec.md`
- `docs/solution.md`
- `docs/operational-contract.md`
- `docs/integration-test-plan.md`

## Operations And Recovery

- Normal operation: run `plan`, then `migrate`, then `validate`.
- Recovery: follow `docs/runbook.md` after a failed migration or validation run.
- Historical baseline: use `rmig baseline --env prod --up-to <VERSION> --confirm` once per existing database.

## Open Issues And Non-Goals

- Open issues: live MSSQL integration validation still needs execution in the target environment.
- Non-goals: `rmig` does not provision SQL Server, manage secrets, or orchestrate the outer CI/CD pipeline.

## References

- `docs/runbook.md`
- `docs/integration-test-plan.md`
