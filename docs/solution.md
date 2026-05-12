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

- Inputs: SQL files under `<RM_SQL_ROOT>/<RM_SQL_BASE>`, `RM_*` environment variables, optional env files loaded through `--env-file` or `RM_ENV_FILE`, command flags, optional approved plan files
- Outputs: stdout plan output, optional `migration-plan.*`, `migration-report.*`, and `validation-report.*` files under `--report-dir`, metadata rows in `[__migrator]`, exit codes, logs
- Ownership boundaries: SQL files are owned in Git; `rmig` owns planning, execution, validation, metadata writes, and report generation

## Assumptions And Constraints

- SQL Server is the execution target.
- SQL Server authentication is selected with `RM_DB_AUTH`. `sql` uses explicit login credentials. `integrated` uses Windows Integrated Security through the MSSQL driver.
- Optional dotenv loading is available through `--env-file` or `RM_ENV_FILE`. It does not run by default and does not replace process environment or CLI flag precedence.
- The env file is trusted operator input, but `rmig` accepts only the supported `RM_*` keys that map to current command inputs. Unknown keys fail before command execution.
- `--env` and `RM_ENV` accept only `pred` and `prod`.
- `RM_SQL_ROOT` and `RM_SQL_BASE` are required for planning, execution, validation, and repair commands.
- If `RM_SQL_BASE` is omitted and `RM_SQL_ROOT` contains exactly one base directory, the runtime uses that directory automatically.
- `RM_SQL_BASE` must be a single directory name under `RM_SQL_ROOT`.
- If `RM_DB_DATABASE` is omitted, the runtime uses `RM_SQL_BASE` as the target database name.
- `plan`, `migrate`, `baseline`, and `repair-checksum` require `RM_GIT_COMMIT`.
- If `RM_GIT_COMMIT` is omitted, the runtime tries to resolve `HEAD` from the nearest `.git` directory above `RM_SQL_ROOT`.
- `plan` writes a stable human-readable text view to stdout by default and writes stable machine-readable JSON to stdout with `--json`. `plan` logs stay on stderr in both modes.
- `--report-dir` or `RM_REPORT_DIR` enables persisted report pairs. Without it, report files are not written to disk.
- `plan` is read-only against SQL Server. It reads metadata state directly and does not bootstrap or repair partial metadata. For tracked table drift, `plan` may create repo-managed transition files under `<RM_SQL_ROOT>/<RM_SQL_BASE>` so the next plan can follow the checked-in transition path. Use `migrate`, `baseline`, or `repair-checksum` when metadata must be bootstrapped under lock.
- Existing module updates are enabled by default for `views`, `procedures`, `functions`, and `triggers`, but only when the repo SQL starts with the matching `CREATE OR ALTER` statement for that object kind.
- Safe tracked table drift is executable automatically when the repo `CREATE TABLE` differs only by additive nullable columns that `rmig` can express as a checked-in transition file.
- When that safe additive drift is detected and no transition file exists yet, `plan` and `migrate` preflight create `<schema>/tables/_migrations/<table>/001_<commit>_auto_add_columns.sql`, rediscover the layout, and continue on the transition-backed path.
- When tracked table drift is not on that safe additive path and no transition file exists yet, `plan` and `migrate` preflight automatically create the technical directory and a scaffold file under `<schema>/tables/_migrations/<table>/001_<commit>_describe_change.sql`.
- Non-safe tracked table drift is executable only when the repo includes checked-in transition scripts under `<schema>/tables/_migrations/<table>/` with file names `<nnn>_<commit>_<slug>.sql` and at least one file no longer carries the scaffold directive.
- When checked-in table transitions exist, the planner exposes their paths in `contracts.PlannedObject.TransitionPaths`, the plan text explains that they will run, and `migrate` executes them from the verified layout.
- For tracked table drift, `migrate` records success for the table object after the transition set and does not replay the repo `tables/*.sql` file as raw create DDL against the existing live table.
- Safe additive transition generation is derived from parsed repo `CREATE TABLE` columns in `internal/parser/table_parser.go` and live table column metadata from `internal/catalog/catalog.go`.
- Planning and managed-scope validation limit catalog reads to schemas present in the resolved repo layout so large unrelated SQL Server schemas are excluded from normal `plan`, `migrate`, `validate`, and `repair-checksum` catalog scans.
- Without a real checked-in table transition, `plan` remains informational, marks the run as blocked, and emits a `transition required` reason that points at the generated scaffold path.
- `RM_TRANSACTION_MODE` defaults to `script`.
- Logs, reports, and stored error text must not expose secrets.
- Post-SQL metadata writes use `internal/migrator/metadata_context.go` and a short timeout so metadata paths fail quickly instead of waiting for the full command timeout.
- When `--report-dir` is set, `internal/reports/write.go` stages report artifacts through temporary files, publishes the text companion first, and publishes JSON last as the commit marker for readers that require a consistent pair.
- Repo-driven `migrate` creates missing schemas, applies create paths and safe existing-module update paths from the current in-memory plan, adopts existing objects without DDL by default, records attempts in `[__migrator]`, and validates the managed object scope by default unless skipped. That post-migrate validation fails closed when any affected managed object is missing, updates `__migrator.items` success state only for the affected object subset from the current migrate plan, and still leaves module refresh work to standalone `validate`.
- When `--plan-file` is supplied, `migrate` reads and validates the approved plan artifact once, rebuilds the current plan from the live catalog and metadata to verify the approval boundary, and skips transition/scaffold generation preflight on that approval path so the repo tree is not mutated during approved execution.
- Repo-driven `validate` refreshes module objects, checks existence for the full managed object scope, creates one validation run row, updates tracked object results for successful validation scope, and writes attempt rows only for validation failures and failed checks.
- Repo-driven `baseline` uses the same discovered schema and object scope as `plan` and `migrate`, creates missing schemas and objects, adopts already existing objects, and blocks when a tracked object already exists with checksum drift. For transition-backed tracked table drift, it stops and directs the operator to `migrate`.
- Repo-driven `baseline` preflights metadata DDL, schema creation permission, object DDL permission, and parent-object availability before create work.
- Repo-driven `repair-checksum` resolves one object by repo path or normalized key, but only when the current plan shows tracked checksum drift for that object and the object is not already on the active transition-backed migrate path. It appends a new successful metadata attempt row in `[__migrator].attempts` instead of mutating old rows in place.
- The append-only metadata history is stored in `[__migrator].attempts`.
- The text plan view explains why each planned object is being created, adopted, skipped, updated, or blocked. It is printed to stdout by default and is persisted as `migration-plan.txt` only when `--report-dir` is set.
- For tracked table drift, that text view includes the checked-in transition paths that `migrate` will apply before the repo table SQL, or the required transition directory when the change is still blocked.
- Metadata bootstrap records runtime schema state in `[__migrator].schema_version`, validates known schema versions before upgrade DDL, and avoids recurring DDL churn on current metadata.
- `inspectMetadataShape()` reads current and legacy metadata objects in one `sys.objects` query instead of per-object `OBJECT_ID()` round-trips.
- After successful metadata bootstrap in locked execution paths, checksum loading uses `LoadSuccessfulChecksums()` directly and skips a second metadata shape inspection.
- Successful checksum reads return only the latest successful action per tracked object key from `[__migrator].attempts` instead of replaying the full append-only attempt history.
- `plan` and repo-driven execution paths reuse one planning catalog read per stage and pass that resolved catalog state into the planner and transition scaffold checks.
- Scope persistence batches `__migrator.items` inserts in chunks of 100 rows and reloads object item ids in one follow-up query per scope transaction.
- Repo layout discovery persists a best-effort file metadata manifest in the local OS cache, keyed by the resolved base path, so unchanged `.sql` files can reuse checksum and directive metadata across later `plan`, `migrate`, and `validate` runs without changing the checked-in repo tree.
- Repo SQL text now loads lazily from `internal/parser/layout.go` on the first caller that needs full script content. Before using lazily loaded content, the runtime rechecks file size, modtime, and checksum and fails closed if the repo changed after discovery.
- `internal/checksum/checksum.go` preserves the existing SQL normalization rules, but file hashing now streams normalized content from disk instead of loading the full file into memory first. `SQLDirHash()` and approved-plan hashing in `internal/migrator/runtime_metadata.go` keep the same digest semantics.
- `internal/migrator/runner.go` emits per-phase duration logs such as `resolve_layout_ms`, `read_catalog_ms`, `build_plan_ms`, `persist_scope_ms`, `execute_plan_ms`, `validate_scope_ms`, and report-write timings so SQL Server latency and repo I/O can be separated during diagnostics.
- Current builds are v2-only for metadata. Legacy metadata objects are treated as incompatible state, and no in-place upgrade path is implemented in `rmig`.

## Nominal Flow

1. Load `RM_*` environment variables and command flags in `internal/app/app.go`.
2. Discover schemas, objects, and checks from `<RM_SQL_ROOT>/<RM_SQL_BASE>` with `internal/parser/layout.go`.
3. Run `rmig plan --env prod --sql-root ./sql --sql-base dwh` to print the operator plan view to stdout.
4. Run `rmig plan --env prod --sql-root ./sql --sql-base dwh --json` when a machine-readable plan artifact is needed on stdout.
5. Add `--report-dir ./reports` to `plan`, `migrate`, or `validate` when persisted report pairs are needed.
6. Run `rmig migrate --env prod --sql-root ./sql --sql-base dwh` to apply the current repo-driven plan and validate the managed object scope.
7. Run `rmig migrate --env prod --sql-root ./sql --sql-base dwh --plan-file ./reports/migration-plan.json` only when approval must be enforced from a saved artifact.
8. Run `rmig validate --env prod --sql-root ./sql --sql-base dwh` to bootstrap metadata if needed, refresh repo-discovered modules, check the full managed object scope, and execute repo-discovered checks.
9. For safe additive tracked table drift, rerun `rmig plan`, confirm it created `<schema>/tables/_migrations/<table>/001_<commit>_auto_add_columns.sql` and now lists that transition path, then run `rmig migrate --env prod --sql-root ./sql --sql-base dwh`.
10. For non-safe tracked table drift, add checked-in transition scripts under `<schema>/tables/_migrations/<table>/001_<commit>_<slug>.sql`, rerun `rmig plan`, and then run `rmig migrate --env prod --sql-root ./sql --sql-base dwh`.
11. Run `rmig baseline --env prod --sql-root ./sql --sql-base dwh --confirm` when an existing database should be brought under repo-driven metadata control without an approved plan artifact.
12. Run `rmig repair-checksum --env prod --sql-root ./sql --sql-base dwh --script reporting/views/monthly.sql --confirm` only when checksum metadata needs controlled repair for one repo-managed object and `migrate` is not the intended path.

## Off-Nominal Behavior And Failure Containment

- Invalid root or base selection: config validation fails before command execution.
- Invalid repository layout: discovery fails before database work.
- Repo files changed after discovery: execution and validation fail closed when a lazily loaded script no longer matches the discovered checksum.
- Partial metadata state during `plan`: the command fails on metadata read errors instead of repairing metadata. Locked metadata bootstrap remains in `migrate`, `baseline`, and `repair-checksum`.
- Blocked `plan`: the command remains informational and read-only even when `Blocked: true`. The hard execution boundary remains in `migrate`.
- Approved-plan drift: when `--plan-file` is supplied, `migrate` fails closed if `git_commit`, `layout_hash`, target, tool identity, comparison mode, update policy, transaction mode, rollback scope, base selection, or the approved schema/object set differs.
- Approved-plan execution does not create transition scaffold files or auto-generated transition files during `migrate --plan-file`; approval mode expects those repo changes to have been made before the artifact was approved.
- Unsafe existing-module update SQL: `plan` blocks the object when the repo file does not start with the required `CREATE OR ALTER` statement.
- Safe additive tracked table drift: `plan` auto-creates `<schema>/tables/_migrations/<table>/001_<commit>_auto_add_columns.sql`, replans onto that checked-in transition path, and `migrate` executes the transition without replaying `tables/*.sql` as raw create DDL.
- Non-safe tracked table drift without transitions: `plan` reports `transition required`, auto-creates the scaffold file, and points at it.
- Scaffold-only tracked table drift: `migrate` fails closed until the scaffold is replaced with real SQL.
- Transition-backed tracked table drift: `migrate` executes the checked-in transition scripts and then records the tracked table update without replaying the repo table file as raw create DDL.
- Metadata failure after SQL success: treated as a critical state in the active repo-driven `migrate`, `baseline`, and `repair-checksum` paths.
- Metadata updates fail closed when the target row is missing or duplicated.
- Legacy metadata state: treated as a breaking compatibility boundary. The runtime stops before bootstrap or checksum reads until the environment is upgraded outside the current CLI.
- Missing schema creation permission, missing object DDL permission, or missing parent object: create paths fail closed with a specific classified error.
- Scope persistence for repo-managed schemas and objects is written into `[__migrator].items` in one metadata transaction per run scope.
- Validation failure: the run stops and writes `validation-report.*` only when `--report-dir` is set.
- Repo-driven migrate execution: the current repo-driven schema/object set is executed. Post-migrate validation checks only the affected object subset from the current migrate plan, but it still fails closed if any affected managed object is missing. When `--plan-file` is supplied, the current set must still match the approved artifact and the approval path skips repo-mutating transition preflight. Repo-discovered checks are outside the `migrate` approval boundary and run only through standalone `validate`.
- Local verification still depends on `staticcheck` being installed on the runner host.

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
