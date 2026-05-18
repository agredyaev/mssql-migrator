# Technical Document: Solution

Lifecycle: `Current`.

## Purpose

Describe the **implemented** `rmig` solution: how the CLI loads configuration, how `internal/engine` orchestrates scan → inspect → diff → (optional) apply, and where artifacts are written. This document is the product-level companion to **`docs/specs/internals/README.md`** (per-package specs).

## Scope

- CLI entry and wiring: `cmd/rmig/main.go`, `internal/app/app.go`, `internal/app/flags.go`, `internal/app/config.go`, `internal/app/wire.go`
- Orchestration: `internal/engine/engine.go`
- Repository scan and layout: `internal/fs/scanner.go`, `internal/fs/layout.go`
- Planning: `internal/diff/diff.go`
- Catalog inspection: `internal/db/inspector_impl.go` (and embedded SQL under `internal/db/sql/`)
- Execution: `internal/apply/apply.go`
- Metadata and audit: `internal/audit/load.go`, `internal/audit/subscriber.go`
- Events and reports: `internal/bus`, `internal/report/report.go`
- Database access boundary: `internal/driver/conn.go`, `internal/driver/mssql`

## System context

`rmig` is a Go binary built from `cmd/rmig`. `main` passes a `driver.Conn` factory into `internal/app.Run`, which parses CLI flags, loads a dotenv-style file, builds `types.Config`, connects to SQL Server, wires `bus.EventBus`, audit/report subscribers, and `engine.Engine`, then runs one command: `plan`, `migrate`, `validate`, `baseline`, or `repair-checksum`.

Schema and object scope come from the SQL tree rooted at **`RM_SQL_ROOT`** (see `(*engine.Engine).runPlan`, which calls `Scanner.Scan(ctx, cfg.SQLRoot)`).

## Interfaces and boundaries

### CLI (`internal/app/flags.go`)

- **Usage:** `rmig [--env <path>] [--json] <command>`
- **`--env <path>`:** path to a dotenv-style file (key=value per line). If the flag is omitted, `internal/app/app.go` still attempts to load the default file named **`.env`** in the current working directory when present.
- **`--json`:** sets JSON structured logs (`cfg.JSONLogs`); it does **not** select machine-readable plan output on stdout (there is no `plan --json` flag today).

### Configuration (`internal/app/config.go`)

Supported keys are read from the env file first, then `os.LookupEnv` for any key not set in the file (see `buildConfig`). **`validateConfig` requires `RM_DB_SERVER`, `RM_DB_DATABASE`, `RM_SQL_ROOT`, and `RM_SQL_BASE`** before a command runs. Other variables (`RM_GIT_COMMIT`, `RM_REPORT_DIR`, …) are optional at validation time unless a subsystem needs them at runtime.

### Reports (`internal/report/report.go`)

When **`RM_REPORT_DIR`** is non-empty, the report subscriber writes under that directory:

- **`.plan.json`** — on `EventDiffComputed` (payload contains `*types.MigrationPlan`).
- **`.report.json`** — on `EventRunFinished` (payload contains `*types.RunFinished`).

There is **no** separate `migration-plan.txt` / `migration-report.json` writer in the current tree; filenames are fixed as above.

### Commands (`internal/engine/engine.go`)

- **`plan`:** `runPlan` → publish `EventDiffComputed` → `EventRunFinished` (success).
- **`migrate`:** `runPlan`; if `plan.Blocked`, `scaffold.Ensure` then return `errors.ErrPlanBlocked`; else acquire session lock, `filterAppliedMigrations`, `applier.Execute`, then finish.
- **`validate`:** `runPlan` (plan used for summary counts), publish validation events, finish.
- **`baseline`** and **`repair-checksum`:** `executeLocked` — same lock + `applier.Execute` path with command name `baseline` or `repair` in bus payloads (see code for exact `RunFinished.Command` values).

## Assumptions and constraints

- SQL Server is the only supported database; the concrete driver is `internal/driver/mssql`.
- `RM_DB_AUTH` follows `internal/types/config.go` (`sql` vs `integrated`).
- Durations `RM_COMMAND_TIMEOUT`, `RM_SCRIPT_TIMEOUT`, `RM_LOCK_TIMEOUT` are parsed with `time.ParseDuration`; invalid values leave the corresponding `time.Duration` at zero (see `buildConfig`).
- **`RM_PLAN_FILE`** and **`RM_REPAIR_SCRIPT`** are read into `types.Config.PlanFile` and `types.Config.RepairTarget` (`internal/app/config.go`). **`internal/engine`** and **`internal/apply`** do not consult these fields yet; they are reserved for a future approved-plan / repair-target gate.

## Nominal flow

1. Operator sets `RM_*` in the process environment or in the file passed to `--env`.
2. Operator runs `rmig --env /path/to/.env plan` (or another command).
3. `engine.runPlan`: `audit.EnsureTables` (once) → `fs.Scanner.Scan` on `RM_SQL_ROOT` → `db.Inspect` → `audit.LoadChecksums` → `diff.Compute`.
4. Subscribers react to bus events; if `RM_REPORT_DIR` is set, `.plan.json` / `.report.json` are updated as described above.
5. `internal/log` writes human-readable lines to stderr (or JSON when `--json`).

## Off-nominal behavior and failure containment

- Config parse errors and missing required `RM_*` keys (`validateConfig`): stderr message, non-zero exit before connect.
- Connect failure: stderr message, non-zero exit (`internal/errors` exit mapping).
- Engine failures: `publishRunFailed` emits `EventRunFinished` with `Result: failure` before returning the error to `app.Run` (see `engine.go`).

## Verification and validation

- `make check` (`Makefile`)
- SQL Server–backed tests: `make test-int` (`internal/app/integration_test.go`, build tag `integration`)

## Operations and recovery

- After a failed run, use **`docs/runbook.md`** with stderr, logs, and (if configured) `.plan.json` / `.report.json` under `RM_REPORT_DIR`.

## Open issues and non-goals

- Open issues: there is **no** `rmig version` subcommand in `cmd/rmig/main.go` today; embedding version requires extending `main` / flags.
- Non-goals: this document does not describe CI/CD outside this repository.

## References

- `README.md`
- `docs/specs/internals/README.md`
- `docs/operational-contract.md`
- `docs/runbook.md`
