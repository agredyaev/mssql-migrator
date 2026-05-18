# Technical Document: Module `internal/app`

Lifecycle: `Current`.

## Purpose

Describe the **CLI composition root**: flag parsing, environment loading, `types.Config` construction, database connection, event bus wiring, and delegation to `internal/engine`.

## Scope

- `internal/app/app.go` — `Run`, `runWithLookup`, command dispatch
- `internal/app/flags.go` — `parseFlags`, `usageText`, valid commands
- `internal/app/config.go` — `buildConfig`, `validateConfig`, supported `RM_*` keys
- `internal/app/wire.go` — `wireEngine`, `attachSubscribers`, `loaderAdapter`

## System context

`cmd/rmig/main.go` passes `app.Run(os.Args, connect)` where `connect` builds `driver.Conn` (typically `mssql.Open`). `app` owns process lifetime: stderr messages on config errors, `defer conn.Close()`, exit codes via `internal/errors` and `internal/types` exit constants.

## Interfaces and boundaries

- Inputs: `os.Args`, process environment, optional dotenv file path from `--env` (default `.env` in `flags.go`)
- Outputs: integer exit code to `os.Exit`; logs via `internal/log` on stderr
- Upstream: none (entry layer)
- Downstream: `internal/driver.Conn`, `internal/bus`, `internal/engine.Engine`, `internal/audit`, `internal/report` subscribers

## Assumptions and constraints

- Assumption: unknown CLI flags or invalid commands fail before network I/O.
- Constraint: `validateConfig` requires **`RM_DB_SERVER`**, **`RM_DB_DATABASE`**, **`RM_SQL_ROOT`**, and **`RM_SQL_BASE`**. Optional: **`RM_PLAN_FILE`**, **`RM_REPAIR_SCRIPT`** (stored on `types.Config`; not yet read by `internal/engine`).
- Constraint: `wire.go` constructs concrete implementations (`fs.NewScanner()`, `db.NewInspector()`, etc.); changing defaults is an `app` change.

## Nominal flow

1. Parse flags and load env file into a string map.
2. `buildConfig` merges env + `os.LookupEnv` (see `config.go` precedence).
3. `validateConfig` rejects incomplete configs.
4. `connect(ctx, cfg)` returns `driver.Conn`.
5. `bus.New()`, `attachSubscribers`, `wireEngine`, `SetBootstrapChecker` on the audit subscriber.
6. Dispatch to `eng.Plan` / `Migrate` / `Validate` / `Baseline` / `RepairChecksum`.
7. Map `execErr` to exit code; log success or error.

## Off-nominal behavior and failure containment

- Parse / env / validation errors: message to stderr, non-zero exit, no DB connection.
- Connection failure: stderr message including server host, exit via `errors.ExitCode`.
- Engine errors: logged at error level, exit via `errors.ExitCode`.

## Verification and validation

- `make check`
- `make test-int` for SQL Server paths (`internal/app/integration_test.go`, build tag `integration`)

## Operations and recovery

- Routine: extend `validCommands` and `switch` in `app.go` when adding a top-level command; update `usageText` and `validateConfig` together.

## Open issues and non-goals

- Non-goals: `internal/app` does not implement planning or SQL execution logic (see `module-engine.md`).

## References

- `internal/engine/engine.go`
- `internal/types/config.go`
- `docs/specs/internals/module-engine.md`
