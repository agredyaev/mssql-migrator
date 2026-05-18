# Technical Document: Operational Contract

Lifecycle: `Current`.

## Purpose

Define how **`rmig`** is built, configured, and operated **as implemented** in this repository: CLI surface, environment contract, artifacts, and failure boundaries.

## Scope

- Build and unit verification: `Makefile`, `README.md`
- CLI: `cmd/rmig/main.go`, `internal/app/flags.go`, `internal/app/config.go`, `internal/app/app.go`
- Runtime wiring: `internal/app/wire.go`, `internal/engine/engine.go`
- Logs: `internal/log/log.go`
- Optional report artifacts: `internal/report/report.go`

## System context

Developers work on a branch, run **`make check`**, then integrate or release a binary built with `go build -o rmig ./cmd/rmig`. Operators invoke `rmig` with database credentials and `RM_SQL_ROOT` pointing at the checked-out SQL tree.

## Interfaces and boundaries

### CLI

- **Invocation:** `rmig [--env <path>] [--json] <command>`
- **Commands:** `plan`, `migrate`, `validate`, `baseline`, `repair-checksum` (`internal/app/flags.go`).
- **`--env`:** filesystem path to a dotenv file containing `KEY=value` lines (comments and blank lines allowed). This is **not** an environment name like `prod` or `pred`; it is strictly a **file path** (see `parseFlags` / `usageText` in `internal/app/flags.go`).
- **`--json`:** enables JSON **logs** to stderr (`cfg.JSONLogs` in `internal/app/config.go`).

### Environment (`RM_*`)

- Values are loaded from the env file (if readable) and overridden by **`os.LookupEnv`** for keys not present in the file (`buildConfig` in `internal/app/config.go`).
- **Hard validation in `validateConfig`:** `RM_DB_SERVER`, `RM_DB_DATABASE`, **`RM_SQL_ROOT`**, and **`RM_SQL_BASE`** must all be non-empty before a command runs.
- Other variables are passed through to `types.Config` when set (for example **`RM_PLAN_FILE`** → `PlanFile`, **`RM_REPAIR_SCRIPT`** → `RepairTarget`); those two are **not** read by `internal/engine` yet but are loaded for forward compatibility.

### Reports

- When **`RM_REPORT_DIR`** is set, `internal/report/report.go` writes **`.plan.json`** and **`.report.json`** into that directory (see `writeJSON` calls). There is no additional `--report-dir` flag; the directory comes from **`RM_REPORT_DIR`** only.

### Outputs

- Primary operator visibility: **stderr logs** via `internal/log`.
- Optional files: `.plan.json`, `.report.json` under `RM_REPORT_DIR`.
- Database side effects: DDL/DML executed by `internal/apply` when commands run apply paths; metadata via `internal/audit` (see package specs).

## Assumptions and constraints

- `go test ./...`, `go vet ./...`, and **`staticcheck`** are part of **`make check`**; the `staticcheck` binary must exist on the developer machine (`Makefile` uses `$(shell go env GOPATH)/bin/staticcheck`).
- There is **no** `rmig version` command in `cmd/rmig/main.go`; version stamping would require new code in `main` or new flags in `internal/app`.
- Flags such as **`--sql-root`**, **`--sql-base`**, **`--plan-file`**, **`--confirm`**, and **`--timeout`** are **not** implemented in `internal/app/flags.go` today; use **`RM_SQL_ROOT`**, **`RM_SQL_BASE`**, and other `RM_*` keys instead. Optional artifact paths: **`RM_PLAN_FILE`** and **`RM_REPAIR_SCRIPT`** populate `types.Config` but are not enforced by engine logic yet. Timeout fields exist on `types.Config` and are populated from **`RM_COMMAND_TIMEOUT`**, **`RM_SCRIPT_TIMEOUT`**, **`RM_LOCK_TIMEOUT`** when parseable.
- The repository does **not** ship a checked-in CI example YAML file under `docs/`; CI is out of scope here.

## Nominal flow

1. `make check`
2. `go build -o rmig ./cmd/rmig`
3. Export or dotenv-file `RM_DB_SERVER`, `RM_DB_DATABASE`, `RM_SQL_ROOT`, `RM_SQL_BASE`, …
4. `rmig --env /path/to/.env plan` (or another command)

## Off-nominal behavior and failure containment

- Unknown CLI flags: process exits before DB connect (`parseFlags` in `internal/app/flags.go`).
- Missing required `RM_DB_*` keys: configuration error on stderr, non-zero exit.
- Engine failures: `EventRunFinished` with failure is published where `publishRunFailed` is used; stderr receives the returned error via `app.Run`.

## Verification and validation

- `make check`
- Optional: `make test-int` for SQL Server integration (`Makefile`, `internal/app/integration_test.go`)

## Operations and recovery

- Use **`docs/runbook.md`** for failure triage with stderr + optional `.plan.json` / `.report.json`.

## Open issues and non-goals

- Open issues: product docs historically assumed extra CLI flags and `rmig version`; those are **not** current behavior until implemented in `cmd/rmig` / `internal/app`.
- Non-goals: this contract does not describe external pipeline promotion rules.

## References

- `README.md`
- `docs/solution.md`
- `docs/runbook.md`
- `docs/specs/internals/README.md`
- `internal/log/log.go`
