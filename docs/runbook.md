# Technical Document: Runbook

Lifecycle: `Current`.

## Purpose

Operator steps after a failed **`rmig`** command: what to read first, which artifacts exist, and how to re-run safely. Aligned with **`internal/app/flags.go`**, **`internal/app/config.go`**, and **`internal/report/report.go`**.

## Scope

- Commands: `plan`, `migrate`, `validate`, `baseline`, `repair-checksum`
- Artifacts: stderr logs from `internal/log`, optional **`.plan.json`** and **`.report.json`** under `RM_REPORT_DIR`
- Configuration: `RM_*` variables (no `--sql-root` / `--sql-base` CLI flags; use env or dotenv file)

## System context

The CLI is invoked as:

```text
rmig [--env <path>] [--json] <command>
```

- **`--env`:** path to the dotenv file whose `RM_*` entries are merged with the process environment (see `internal/app/app.go` / `config.go`). If omitted, a default **`.env`** load is still attempted when that file exists.
- **`--json`:** JSON logs to stderr only.

Repository and database selection use **`RM_SQL_ROOT`**, **`RM_SQL_BASE`**, **`RM_DB_*`**, and related keys—set them in the environment or in the env file.

## Interfaces and boundaries

- Inputs: stderr output, optional `RM_REPORT_DIR` contents (`.plan.json`, `.report.json`), SQL Server error text, Git state under `RM_SQL_ROOT`
- Outputs: corrected SQL or config, then a clean rerun of the same command
- Ownership: Git owns repo SQL; operators own database credentials and env files

## Assumptions and constraints

- **`validateConfig`** (in `internal/app/config.go`) requires **`RM_DB_SERVER`**, **`RM_DB_DATABASE`**, **`RM_SQL_ROOT`**, and **`RM_SQL_BASE`** before connect.
- There is **no** `--confirm` flag in the current CLI; any “confirm” behavior described in older docs is **not** implemented at the flag layer.
- There is **no** `--plan-file` flag; optional **`RM_PLAN_FILE`** is loaded into `types.Config` but **`internal/engine`** does not enforce it yet.

## Nominal flow

1. Capture **stderr** (and stdout if anything is printed by future changes).
2. If `RM_REPORT_DIR` was set, open **`<RM_REPORT_DIR>/.plan.json`** for the last computed plan and **`<RM_REPORT_DIR>/.report.json`** for the last `EventRunFinished` payload.
3. Fix the underlying issue (SQL, layout, permissions, metadata) in Git or the database as appropriate.
4. Re-run the same `rmig` command with the same `--env` file after review.

## Off-nominal behavior and failure containment

- **`plan` errors:** read stderr; inspect `.plan.json` if present for `Blocked`, `Blockers`, and object actions.
- **`migrate` blocked:** engine returns `errors.ErrPlanBlocked` after `scaffold.Ensure` (see `internal/engine/engine.go`); repo may contain new scaffold files—commit or delete them per your process before the next migrate.
- **`migrate` apply errors:** stderr lists failed objects; `.report.json` may still record `RunFinished` with failure (see bus publish ordering in `engine.go`).
- **Lock acquisition errors:** migrate / baseline / repair stop before apply; no partial apply from engine for that failure mode (see `locker.Acquire`).

## Verification and validation

Sanity after a change:

```bash
make check
```

SQL Server smoke (Docker):

```bash
make test-int
```

## Operations and recovery

- Prefer **small reruns** (`plan` only) before `migrate` when diagnosing layout or metadata issues.
- Treat **`.plan.json`** as the structured view of the last diff computation for the run process; regenerate by rerunning `plan` or any command that calls `runPlan`.

## Open issues and non-goals

- Non-goals: this runbook does not define change-management approval outside Git and your pipeline.

## References

- `README.md`
- `docs/solution.md`
- `docs/operational-contract.md`
- `internal/engine/engine.go`
- `internal/report/report.go`
