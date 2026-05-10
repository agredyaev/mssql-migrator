# rmig

Lifecycle: `Current`.

## Purpose

`rmig` is the Go CLI contract for MSSQL reporting-layer planning, execution, validation, and metadata capture.
It operates on the repo-driven v8 model under `<RM_SQL_ROOT>/<RM_SQL_BASE>`.

## Scope

- Runtime dispatch: `internal/app/app.go`
- CLI command handlers: `internal/migrator/handler.go`, `internal/migrator/runner.go`, `internal/migrator/baseline_repair.go`, `internal/migrator/validation.go`
- Metadata store: `internal/metadata/metadata.go`
- Planning logic: `internal/planner/planner.go`
- Repo layout discovery: `internal/parser/layout.go`
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
Use `--env pred` for pre-production validation runs and `--env prod` for production runs.
The tool reads repo-driven SQL files from `<RM_SQL_ROOT>/<RM_SQL_BASE>`, writes reports to `./reports`, and persists run state in `[__migrator].schema_version`, `[__migrator].runs`, `[__migrator].items`, and the append-only `[__migrator].attempts` log.

## Interfaces And Boundaries

- Inputs: `RM_DB_SERVER`, `RM_DB_PORT`, `RM_DB_DATABASE`, `RM_DB_AUTH`, `RM_DB_USER`, `RM_DB_PASSWORD`, `RM_GIT_COMMIT`, `RM_GIT_BRANCH`, `RM_PIPELINE_RUN_ID`, `RM_PIPELINE_URL`, `RM_ACTOR`, `RM_ENV_FILE`, `RM_SQL_ROOT`, `RM_SQL_BASE`, `RM_UPDATE_POLICY`, `RM_TRANSACTION_MODE`, `RM_PLAN_JSON`, `--env`, `--env-file`, `--sql-root`, `--sql-base`, `--plan-file`, `--json`, `--update-policy`, `--transaction-mode`, `--script`, `--confirm`, `--skip-validate`, SQL files under `<RM_SQL_ROOT>/<RM_SQL_BASE>`
- Outputs: `reports/migration-plan.json`, `reports/migration-plan.txt`, `reports/migration-report.json`, `reports/migration-report.txt`, `reports/validation-report.json`, `reports/validation-report.txt`, metadata rows in `[__migrator]`, exit codes, logs
- Ownership boundaries: SQL Server access is external; `rmig` owns planning, execution, validation, report generation, and writes only inside `[__migrator]`

## Flags And Env

- Common flags: `--env` (`pred` or `prod`), `--sql-root`, `--sql-base`, `--report-dir`, `--log-level`, `--json-logs`, `--timeout`, `--script-timeout`, `--lock-timeout`
- Plan flags: `--json`, `--update-policy`, `--transaction-mode`
- Migrate flags: `--plan-file`, `--skip-validate`
- Repair flag: `--script` selects one repo object path or normalized key for `repair-checksum`
- Optional env file flag: `--env-file` loads supported `RM_*` values from a dotenv-style file. The file is ignored unless `--env-file` or `RM_ENV_FILE` is set.
- The env file is trusted operator input, but `rmig` accepts only the supported `RM_*` keys that map to current command inputs. Unknown keys and non-`RM_*` keys fail validation with a line-number error.
- Supporting environment: `RM_ENV`, `RM_SQL_ROOT`, `RM_SQL_BASE`, `RM_REPORT_DIR`, `RM_LOG_LEVEL`, `RM_JSON_LOGS`, `RM_TIMEOUT`, `RM_SCRIPT_TIMEOUT`, `RM_LOCK_TIMEOUT`, `RM_PLAN_FILE`, `RM_REPAIR_SCRIPT`, `RM_SKIP_VALIDATE`, `RM_CONFIRM`, `RM_ENV_FILE`, `RM_UPDATE_POLICY`, `RM_TRANSACTION_MODE`, `RM_PLAN_JSON`
- Database authentication environment: `RM_DB_AUTH` (`sql` or `integrated`). `sql` is the default and requires `RM_DB_USER` and `RM_DB_PASSWORD`. `integrated` uses Windows Integrated Security through the driver `winsspi` authenticator. `RM_DB_USER` is optional in integrated mode and can be omitted to use the current Windows session.
- Precedence when `--env-file` or `RM_ENV_FILE` is used: CLI flags override process environment, process environment overrides `.env`, and `.env` overrides built-in defaults.

## Assumptions And Constraints

- SQL Server is reachable with the configured credentials.
- `RM_DB_AUTH=sql` requires `RM_DB_USER` and `RM_DB_PASSWORD`.
- `RM_DB_AUTH=integrated` uses Windows Integrated Security and does not require `RM_DB_PASSWORD`.
- `.env` loading is opt-in only. Without `--env-file` or `RM_ENV_FILE`, startup behavior stays unchanged.
- `--env` and `RM_ENV` accept only `pred` and `prod`.
- `RM_SQL_ROOT` and `RM_SQL_BASE` are required for `plan`, `migrate`, `validate`, `baseline`, and `repair-checksum`.
- If `RM_SQL_BASE` is omitted and `RM_SQL_ROOT` contains exactly one base directory, `rmig` uses that directory automatically.
- `RM_SQL_BASE` must be a single directory name under `RM_SQL_ROOT`.
- `plan`, `migrate`, `baseline`, and `repair-checksum` require `RM_GIT_COMMIT`.
- If `RM_GIT_COMMIT` is omitted, `rmig` tries to read `HEAD` from the nearest `.git` directory above `RM_SQL_ROOT`.
- `plan --json` writes machine-readable JSON to stdout. Human logs go to stderr for that mode.
- `plan` is read-only. It reads metadata state directly and does not bootstrap or repair partial metadata. Use `migrate`, `baseline`, or `repair-checksum` when metadata must be bootstrapped under the session lock.
- `RM_UPDATE_POLICY` supports `none`, `modules_only`, and `all_supported`. Existing module updates are allowed only when the repo SQL starts with the matching `CREATE OR ALTER` statement for that object kind.
- `RM_TRANSACTION_MODE` supports `script` and `none`.
- Logs, reports, and stored error text must not expose secrets.
- `migrate` executes approved create paths and safe existing-module update paths after approved-plan verification, treats `adopt_existing` as a no-DDL adoption path, records attempts into `[__migrator]`, and limits post-migrate validation to managed-scope existence and metadata checks without module refresh work.
- `baseline` uses the same repo-driven layout as `plan` and `migrate`. It creates missing repo-managed schemas and objects, adopts already existing objects without DDL, and fails closed on checksum drift.
- `baseline` preflights metadata DDL, schema creation permission, object DDL permission, and missing parent objects before create work.
- `repair-checksum` targets one repo object selected by path or normalized key, but only when the current plan shows tracked checksum drift for that object. It appends a new successful metadata attempt row in `[__migrator].attempts` instead of editing old checksum history in place.
- The text form of `migration-plan.txt` explains why each object is planned for create, adopt, skip, update, or block so operators do not need to infer planner state from action codes alone.
- Report files are written through `internal/reports/write.go` by staging `*.tmp` files, publishing the text companion first, and publishing JSON last as the commit marker for readers that require a consistent pair.
- Metadata writes use a short bounded context in `internal/migrator/metadata_context.go` so post-SQL metadata updates do not hang until the full command timeout.
- Catalog reads are shared through `internal/catalog/catalog.go` so plan and validation classify the live SQL Server object set the same way.
- Metadata bootstrap records schema version state in `[__migrator].schema_version`, validates known schema versions before upgrade DDL, and does not churn existing indexes and view definitions on every run.

## Nominal Flow

1. Build with `go build -ldflags "-X main.version=0.1.0-dev -X main.commit=$(git rev-parse HEAD)" -o rmig ./cmd/rmig`.
2. Run `rmig plan --env prod --sql-root ./sql --sql-base dwh` to write `reports/migration-plan.json` and `reports/migration-plan.txt`.
3. Run `rmig plan --env prod --sql-root ./sql --sql-base dwh --json` when a machine-readable plan payload is required on stdout. Do not redirect it over `reports/migration-plan.json`, because `plan` already writes that file.
4. Run `rmig migrate --env prod --sql-root ./sql --sql-base dwh --plan-file reports/migration-plan.json` to verify the approved plan, create missing schemas, apply planned repo objects, record `adopt_existing` metadata rows, and validate the managed object scope.
5. Run `rmig validate --env prod --sql-root ./sql --sql-base dwh` to bootstrap metadata if needed, refresh repo-discovered module objects, and execute repo-discovered check scripts.
6. Run `rmig baseline --env prod --sql-root ./sql --sql-base dwh --confirm` to create or adopt the current repo-managed scope without an approved plan artifact.
7. Run `rmig repair-checksum --env prod --sql-root ./sql --sql-base dwh --script reporting/views/monthly.sql --confirm` only when the current plan shows tracked checksum drift for that repo object.

## Off-Nominal Behavior And Failure Containment

- Invalid SQL root or base selection: command validation fails before database work.
- Invalid repository layout: `plan` or `validate` fails before object work.
- Repository schema, object, and parent identifiers must match `^[A-Za-z_][A-Za-z0-9_@$#]*$`.
- Bracketed, spaced, hyphenated, and Unicode SQL identifiers are not supported in repository paths.
- Partial metadata state during `plan`: `plan` stays read-only and fails on metadata read errors instead of repairing state. Use `migrate`, `baseline`, or `repair-checksum` to bootstrap metadata under lock.
- Approved-plan drift: `migrate` rejects the plan if `git_commit`, `layout_hash`, target, tool identity, comparison mode, update policy, transaction mode, rollback scope, base selection, or the approved schema/object set differs.
- Metadata read or write failure: the run reports a critical state and stops. Metadata updates also fail closed when the target row is missing or duplicated.
- Unsafe existing-module update SQL: `plan` blocks the object when the repo file does not start with the required `CREATE OR ALTER` statement.
- Missing schema creation permission or missing object DDL permission: `baseline` or `migrate` stops with a permission-specific error.
- Missing parent object for `indexes` or `triggers`: execution stops with a parent-object failure.
- Validation failure: the run stops and writes `reports/validation-report.*`.
- Repo-discovered `checks/*.sql` run only in standalone `validate`, not inside `migrate` or `baseline`.

## Verification And Validation

- `PATH=/usr/local/go/bin:$PATH go test ./...`
- `PATH=/usr/local/go/bin:$PATH test -z "$(gofmt -l .)"`
- `PATH=/usr/local/go/bin:$PATH go test -race ./...`
- `PATH=/usr/local/go/bin:$PATH staticcheck ./...`
- `RMIG_RUN_SQLSERVER_INTEGRATION=1 PATH=/usr/local/go/bin:$PATH go test ./internal/migrator -run SQLServer`
- `PATH=/usr/local/go/bin:$PATH go vet ./...`
- `PATH=/usr/local/go/bin:$PATH go build -ldflags "-X main.version=0.1.0-dev -X main.commit=$(git rev-parse HEAD)" -o rmig ./cmd/rmig`
- `./rmig version`
- Review dumps produced with `./export_code_to_txt.sh` must include `go.mod` and `go.sum` together with `cmd/**` and `internal/**` so dependency resolution can be validated from the artifact.
- Unit and contract tests in `internal/app/app_test.go`, `internal/config/config_test.go`, `internal/logger/logger_test.go`, `internal/planner/planner_test.go`, `internal/parser/*_test.go`, `internal/migrator/*_test.go`, `internal/checksum/checksum_test.go`, `internal/validate/validate_test.go`, `internal/reports/report_schema_test.go`
- Documentation governance: `docs/specs/documentation-spec.md` and `docs/specs/nasa-document-spec.md`

## Operations And Recovery

- Normal operation: run `plan`, then `migrate`, then `validate`.
- Recovery: follow `docs/runbook.md` after a failed planning, migration, validation, baseline, or repair run.
- Use `rmig baseline` when the repo layout already describes the desired target state and the database should be created or adopted into current repo-driven metadata.
- Use `rmig repair-checksum` only when one repo-managed object is already tracked and the current plan shows checksum drift for that object.

## Open Issues And Non-Goals

- Open issues: live MSSQL integration validation is opt-in and still depends on an external disposable SQL Server.
- Non-goals: `rmig` does not provision SQL Server, manage secrets, or orchestrate the outer CI/CD pipeline.

## References

- `docs/runbook.md`
- `docs/integration-test-plan.md`
