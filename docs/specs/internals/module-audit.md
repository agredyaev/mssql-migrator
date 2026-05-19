# Technical Document: Module `internal/audit`

Lifecycle: `Current`.

## Purpose

Describe **metadata persistence**: ensure migrator tables exist, load checksum history and applied migrations, and record apply outcomes to history after each run.

## Scope

- `internal/audit/load.go` — `EnsureTables`, `LoadChecksums` (OpenJSON only), `LoadAllAppliedMigrations`
- `internal/audit/subscriber.go` — `Subscriber`, batched history insert on `EventRunFinished`
- `internal/audit/sql/` — embedded DDL/DML (`.sql` files)

## System context

`internal/app/app.go` calls `audit.EnsureTables` once after connect. `internal/app/wire.go` uses `loaderAdapter` for `engine` loaders. `audit.NewSubscriber` buffers `EventObjectApplied` / `EventObjectFailed` and **flushes** to SQL on `EventRunFinished` (decouples apply hot path from audit I/O).

## Interfaces and boundaries

- Inputs: `driver.Conn`, `context.Context`, bus event payloads
- Outputs: SQL side effects in `azdo_deploy_meta` tables (see embed SQL)
- Downstream: none (leaf for persistence); upstream: `engine`, `apply`

## Assumptions and constraints

- Assumption: migrator schema matches embedded DDL in `EnsureTables`.
- Constraint: `LoadChecksums` and history insert use **OpenJSON** SQL only (`load_checksums_openjson.sql`, `insert_history_openjson.sql`).
- Constraint: checksum caches invalidate on history write (`bumpChecksumsCacheGeneration`); catalog cache invalidates on history flush when `RMIG_CATALOG_CACHE` is enabled (`db.InvalidateCatalogCacheForConn`).

## Nominal flow

1. `EnsureTables` at app startup (and subscriber `boot` on first flush if needed).
2. `LoadChecksums` during `runPlan` (may run in parallel with `db.Inspect`).
3. Apply publishes object events; subscriber **enqueues** records.
4. On `EventRunFinished`, subscriber writes one OpenJSON batch insert.

## Off-nominal behavior and failure containment

- Bootstrap errors are surfaced via `BootstrapChecker` to engine (`SetBootstrapChecker` in `app.go`).
- Subscriber notifies optional `ErrorNotifier` on unexpected payloads or insert errors (warn path).

## Verification and validation

- `make check` (`internal/audit/audit_test.go`)
- Integration tests with SQL Server when `RMIG_RUN_SQLSERVER_INTEGRATION=1`

## Operations and recovery

- Schema migrations for `azdo_deploy_meta` require coordinated changes to embed SQL, Go scanners, and tests.

## Open issues and non-goals

- Non-goals: audit does not compute diffs.

## References

- `internal/bus/payload.go`
- `docs/specs/internals/module-bus.md`
