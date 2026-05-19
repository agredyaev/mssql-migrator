# Technical Document: Operational Contract

Lifecycle: `Current`.

## Purpose

Define how **`rmig`** is built, configured, and operated **as implemented** in this repository: CLI surface, environment contract, artifacts, and failure boundaries.

## Scope

- Build and unit verification: `Makefile`, `README.md`
- CLI: `cmd/rmig/main.go`, `internal/app/flags.go`, `internal/app/config.go`, `internal/app/app.go`
- Version metadata: `internal/buildinfo/buildinfo.go`, root **`VERSION`**, `Makefile` (`release-build`)
- Runtime wiring: `internal/app/wire.go`, `internal/engine/engine.go`
- Logs: `internal/log/log.go`
- Optional report artifacts: `internal/report/report.go`

## System context

Developers work on a branch, run **`make check`**, then integrate or release a binary built with `go build -o rmig ./cmd/rmig`. Operators invoke `rmig` with database credentials and `RM_SQL_ROOT` pointing at the checked-out SQL tree.

## Interfaces and boundaries

### CLI

- **Invocation:** `rmig [--env <path>] [--json] <command>`
- **Commands:** `plan`, `migrate`, `validate`, `baseline`, `repair-checksum`, **`version`** (`internal/app/flags.go`). The **`version`** command prints semver and a short commit to **stdout** and exits before reading `.env`, before `validateConfig`, and before database connect (`internal/app/app.go`).
- **`--env`:** filesystem path to a dotenv file containing `KEY=value` lines (comments and blank lines allowed). This is **not** an environment name like `prod` or `pred`; it is strictly a **file path** (see `parseFlags` / `usageText` in `internal/app/flags.go`). Ignored for **`version`** (that command does not load a file).
- **`--json`:** enables JSON **logs** to stderr (`cfg.JSONLogs` in `internal/app/config.go`) for engine commands; for **`version`**, writes one JSON object to stdout with `version` and `commit` keys.

### Release binary version stamping (`Makefile`, `internal/buildinfo`)

- Root file **`VERSION`** holds the semver string consumed by **`make release-build`**.
- **`make release-build`** passes `-ldflags "-X reporting-db-migrations/internal/buildinfo.Version=… -X reporting-db-migrations/internal/buildinfo.Commit=…"` (see `Makefile`).

### Environment (`RM_*`)

- Values are loaded from the env file (if readable) and overridden by **`os.LookupEnv`** for keys not present in the file (`buildConfig` in `internal/app/config.go`).
- **Hard validation in `validateConfig`:** `RM_DB_SERVER`, `RM_DB_DATABASE`, **`RM_SQL_ROOT`**, and **`RM_SQL_BASE`** must all be non-empty before a command runs.
- Other variables are passed through to `types.Config` when set (for example **`RM_PLAN_FILE`** → `PlanFile`, **`RM_REPAIR_SCRIPT`** → `RepairTarget`); those two are **not** read by `internal/engine` yet but are loaded for forward compatibility.

### Reports

- When **`RM_REPORT_DIR`** is set, `internal/report/report.go` writes **`.plan.json`** and **`.report.json`** into that directory (see `writeJSON` calls). There is no additional `--report-dir` flag; the directory comes from **`RM_REPORT_DIR`** only.

### Outputs

- Primary operator visibility: **stderr logs** via `internal/log`.
- **`rmig version`:** one line to **stdout** (or JSON to stdout when `--json`); no stderr log line on success.
- Optional files: `.plan.json`, `.report.json` under `RM_REPORT_DIR`.
- Database side effects: DDL/DML executed by `internal/apply` when commands run apply paths; metadata via `internal/audit` (see package specs).

## Assumptions and constraints

- `go test ./...`, `go vet ./...`, and **`staticcheck`** are part of **`make check`**; the `staticcheck` binary must exist on the developer machine (`Makefile` uses `$(shell go env GOPATH)/bin/staticcheck`).
- **`rmig version`** is implemented in **`internal/app`** with link-time metadata in **`internal/buildinfo`**; **`RM_TOOL_VERSION`** remains a separate operator-supplied value for reports (`internal/app/config.go`), not the CLI’s embedded version string.
- Flags such as **`--sql-root`**, **`--sql-base`**, **`--plan-file`**, **`--confirm`**, and **`--timeout`** are **not** implemented in `internal/app/flags.go` today; use **`RM_SQL_ROOT`**, **`RM_SQL_BASE`**, and other `RM_*` keys instead. Optional artifact paths: **`RM_PLAN_FILE`** and **`RM_REPAIR_SCRIPT`** populate `types.Config` but are not enforced by engine logic yet. Timeout fields exist on `types.Config` and are populated from **`RM_COMMAND_TIMEOUT`**, **`RM_SCRIPT_TIMEOUT`**, **`RM_LOCK_TIMEOUT`** when parseable.
- The repository does **not** ship a checked-in CI example YAML file under `docs/`; CI is out of scope here.

## Nominal flow

1. `make check`
2. `go build -o rmig ./cmd/rmig` (optional: `make release-build` to stamp **`VERSION`** / git commit into `bin/rmig`)
3. `rmig version` (optional sanity check; no `RM_*` required)
4. Export or dotenv-file `RM_DB_SERVER`, `RM_DB_DATABASE`, `RM_SQL_ROOT`, `RM_SQL_BASE`, …
5. `rmig --env /path/to/.env plan` (or another engine command)

## Off-nominal behavior and failure containment

- Unknown CLI flags: process exits before DB connect (`parseFlags` in `internal/app/flags.go`).
- Missing required `RM_DB_*` keys: configuration error on stderr, non-zero exit (does not apply to **`version`**).
- Engine failures: `EventRunFinished` with failure is published where `publishRunFailed` is used; stderr receives the returned error via `app.Run`.

## Verification and validation

- `make check`
- `rmig version` / `make release-build && ./bin/rmig version`
- Optional: `make test-int` for SQL Server integration (`Makefile`, `internal/app/integration_test.go`)

## Operations and recovery

- Use **`docs/runbook.md`** for failure triage with stderr + optional `.plan.json` / `.report.json`.

## Open issues and non-goals

- Open issues: none for `rmig version` / `internal/buildinfo` beyond maintaining **`VERSION`** and release build flags.
- Non-goals: this contract does not describe external pipeline promotion rules.

## References

- `README.md`
- `docs/solution.md`
- `docs/runbook.md`
- `docs/specs/internals/README.md`
- `internal/log/log.go`
