# Technical Document: Operational Contract

Lifecycle: `Current`.

## Purpose

This document defines how the current `rmig` build is configured, validated, and operated on the repo-driven v8 runtime path.

## Scope

- Local build commands in `README.md`
- CI example in `docs/ci-example.yml`
- Secret redaction in `internal/logger/*.go`
- Runtime metadata injection in `internal/app/app.go`
- Planning and validation behavior in `internal/planner/*.go`, `internal/parser/layout.go`, and `internal/validate/*.go`

## System Context

The intended workflow is: develop on a branch, open a PR to `main`, pass compile and test checks, then run a production job that invokes `rmig` against SQL Server.
The production job must pass a real commit SHA into the binary and must not leak secrets in logs or reports.
See `README.md` for the CLI wrapper contract.

## Interfaces And Boundaries

- Inputs: `RM_*` environment variables, optional env files selected through `--env-file` or `RM_ENV_FILE`, SQL Server connection details, `--sql-root`, `--sql-base`, optional `--report-dir`, optional `--plan-file`, `--json`, `--transaction-mode`, build metadata
- Outputs: plan output on stdout, optional `migration-plan.*`, `migration-report.*`, and `validation-report.*` files under `--report-dir`, SQL Server metadata rows in `[__migrator]`, exit codes, logs
- Ownership boundaries: the repository owns `rmig`; the release pipeline owns promotion and deployment timing

## Assumptions And Constraints

- `go test ./...`, `go vet ./...`, and a release build of the CLI wrapper are required before promotion.
- The binary is built with `-ldflags` so `rmig version` reports a real commit SHA.
- `baseline` and `repair-checksum` require `--confirm`.
- `plan`, `migrate`, `baseline`, and `repair-checksum` require `RM_GIT_COMMIT`.
- If `RM_GIT_COMMIT` is omitted, the runtime tries to resolve `HEAD` from the nearest `.git` directory above `RM_SQL_ROOT`.
- `--env` and `RM_ENV` accept only `pred` and `prod`.
- `RM_SQL_ROOT` and `RM_SQL_BASE` must point to unpacked repository files on disk.
- If `RM_SQL_BASE` is omitted and `RM_SQL_ROOT` contains exactly one base directory, the runtime uses that base automatically.
- `RM_SQL_BASE` must be a single directory name under `RM_SQL_ROOT`.
- If `RM_DB_DATABASE` is omitted, the runtime uses `RM_SQL_BASE` as the target database name.
- `plan` emits the human-readable operator view to stdout by default. `plan --json` emits machine-readable JSON to stdout. `plan` logs stay on stderr in both modes.
- `--report-dir` or `RM_REPORT_DIR` enables persisted report files. Without it, `rmig` does not write report files to disk.
- `plan` is read-only. It reads metadata state directly and does not bootstrap or repair partial metadata. Use `migrate`, `baseline`, or `repair-checksum` when metadata must be bootstrapped under lock.
- SQL Server authentication settings are provided externally. `RM_DB_AUTH=sql` uses `RM_DB_USER` and `RM_DB_PASSWORD`. `RM_DB_AUTH=integrated` uses the current Windows session or an explicit Windows user value passed in `RM_DB_USER`.
- Optional dotenv loading must be explicitly enabled. When enabled, CLI flags still win over process environment, and process environment still wins over values loaded from the env file.
- The env file may contain only supported `RM_*` keys for current command inputs. Unknown keys and non-`RM_*` keys fail validation.
- Malformed `RM_TIMEOUT`, `RM_SCRIPT_TIMEOUT`, and `RM_LOCK_TIMEOUT` values, including values loaded through `--env-file`, are configuration errors. Malformed `--timeout`, `--script-timeout`, and `--lock-timeout` flag values are invalid input.
- Repo-driven object execution runs from the current in-memory plan. When `--plan-file` is supplied, approved-plan verification must succeed before execution. Post-migrate validation is limited to managed-scope existence and metadata checks without module refresh work.
- `baseline` uses the repo-driven layout, creates missing schemas and objects, adopts existing objects, and requires `--confirm`. It is not the execution path for transition-backed tracked table changes.
- `baseline` fails closed on missing metadata DDL permission, missing schema creation permission, missing object DDL permission, checksum drift, or missing parent objects.
- `repair-checksum` uses one repo object selected by `--script <repo-path-or-normalized-key>`, but only when the current plan shows tracked checksum drift for that object and the drift is not on the active transition-backed migrate path. It appends a new successful metadata attempt row in `[__migrator].attempts` and does not rewrite old checksum history.
- Current builds do not migrate legacy metadata objects such as `__migrator.migration_runs` or `__migrator.migration_attempts` to the current schema. This is a breaking migration requirement for existing environments: legacy metadata must be upgraded or removed outside the current CLI before v2-only builds can run. If any legacy metadata object is present, bootstrap and checksum reads fail closed with `metadata_schema_incompatible`.
- In operator-facing wording, `[__migrator].attempts` is the append-only attempt log, and `[__migrator].items` is the per-run managed scope snapshot for schemas and objects.
- The text plan view is the human-oriented operator view. It is printed to stdout by default and persisted as `migration-plan.txt` only when `--report-dir` is set. It explains why each object is planned for create, adopt, skip, update, or block.
- When `--report-dir` is set, `internal/reports/write.go` publishes the text companion first and the JSON file last. The JSON artifact is the commit marker for readers that require a consistent report pair.
- Existing module updates are enabled by default for `views`, `procedures`, `functions`, and `triggers`, but only when the repo SQL starts with the matching `CREATE OR ALTER` statement for that object kind.
- Tracked table drift is executable only through checked-in transition scripts discovered under `<schema>/tables/_migrations/<table>/` with file names `<nnn>_<commit>_<slug>.sql`.
- Without a checked-in table transition, `plan` remains informational, reports `Blocked: true`, and `migrate` fails closed.

## Nominal Flow

1. Run local verification: `go test ./...`, `go vet ./...`, `go build -ldflags "-X main.version=0.1.0-dev -X main.commit=$(git rev-parse HEAD)" -o rmig ./cmd/rmig`.
2. Run `rmig version` to confirm the version and commit.
3. Run `rmig plan --env prod --sql-root ./sql --sql-base dwh` or `rmig plan --env prod --sql-root ./sql --sql-base dwh --json`.
4. Add `--report-dir ./reports` when persisted report pairs are required.
5. Run `rmig migrate --env prod --sql-root ./sql --sql-base dwh`.
6. Run `rmig migrate --env prod --sql-root ./sql --sql-base dwh --plan-file ./reports/migration-plan.json` only when an approved plan artifact must be enforced.
7. Run `rmig validate --env prod --sql-root ./sql --sql-base dwh`.
8. For tracked table drift, add checked-in transition scripts under `<schema>/tables/_migrations/<table>/001_<commit>_<slug>.sql`, rerun `rmig plan`, and then run `rmig migrate --env prod --sql-root ./sql --sql-base dwh`.
9. Run `rmig baseline --env prod --sql-root ./sql --sql-base dwh --confirm` only when the current repo layout is already the intended target state.
10. Run `rmig repair-checksum --env prod --sql-root ./sql --sql-base dwh --script reporting/views/monthly.sql --confirm` only when the runbook allows controlled checksum repair and `migrate` is not the intended execution path.

## Off-Nominal Behavior And Failure Containment

- Secret leakage: redaction must strip password and token-like values from logs, reports, and stored error text.
- Invalid root or base selection: the command fails before any database work.
- Invalid repository layout: planning or validation fails before object work.
- Partial metadata state during `plan`: the command fails on metadata read errors instead of repairing metadata. Use `migrate`, `baseline`, or `repair-checksum` for metadata bootstrap under lock.
- Blocked `plan`: the command remains informational and read-only. Operators must treat the block reasons as the decision point for the next step, while `migrate` remains the enforcing path.
- Plan drift: when `--plan-file` is supplied, `migrate` checks the approved plan and fails closed if `git_commit`, `layout_hash`, target, tool identity, comparison mode, update policy, transaction mode, rollback scope, base selection, or the approved schema/object set changed.
- Unsafe existing-module update SQL: `plan` blocks the object before execution.
- Tracked table drift without transitions: `plan` reports `transition required` and names the required `<schema>/tables/_migrations/<table>/` path.
- Transition-backed table drift: `migrate` executes the checked-in transition set before the repo table SQL from the verified layout.
- Metadata failure: the run stops instead of reporting success. Metadata updates also fail closed when the target row is missing or duplicated.
- Legacy metadata shape: current builds stop before metadata bootstrap or checksum reads. Operators must upgrade or remove legacy metadata outside the current CLI because no in-place v1 to v2 migration path exists yet.
- Repo-driven migrate execution: current builds execute the current repo-driven schema/object set, optionally write migration reports when `--report-dir` is set, record `adopt_existing` into metadata, and run managed-scope validation by default unless skipped. When `--plan-file` is supplied, the current set must match the approved artifact. Repo-discovered `checks/*.sql` stay outside `migrate` and run only in standalone `validate`.
- Baseline create path: current builds preflight DDL permissions, create missing repo-managed scope, record item and attempt rows into `[__migrator]`, and stop on the first classified failure.
- Validation failure: the run stops and writes `validation-report.*` only when `--report-dir` is set.

## Verification And Validation

- See `README.md` for shared build and unit-test commands.
- `docs/integration-test-plan.md`

## Operations And Recovery

- Use `docs/runbook.md` for planning failure, validation failure, and metadata repair.
- Re-run `rmig plan` after any metadata repair.
- If `--report-dir` is used for signoff artifacts, retain `migration-plan.*`, `migration-report.*`, and `validation-report.*` with the release record.

## Open Issues And Non-Goals

- Open issues: live production verification is still required before claiming full production proof.
- Non-goals: this document does not describe the external CI provider configuration.

## References

- `README.md`
- `docs/solution.md`
- `docs/runbook.md`
- `internal/logger/logger.go`
