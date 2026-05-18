# Technical Document: Module `internal/audit`

Lifecycle: `Current`.

## Purpose

Describe **metadata persistence**: ensure migrator tables exist, load checksum history and applied migrations, and subscribe to apply events to insert audit rows.

## Scope

- `internal/audit/load.go` — `EnsureTables`, `LoadChecksums`, `LoadAllAppliedMigrations`
- `internal/audit/subscriber.go` — `Subscriber`, `NewSubscriber`, batch inserts (`//go:embed sql/...`)
- `internal/audit/sql/` — embedded DML/DDL (`.sql` files)

## System context

`internal/app/wire.go` uses `loaderAdapter` to expose audit loaders to `engine`. `audit.NewSubscriber` attaches to the same `bus.EventBus` as apply for `EventObjectApplied` / `EventObjectFailed`.

## Interfaces and boundaries

- Inputs: `driver.Conn`, `context.Context`, event payloads from `internal/bus`
- Outputs: SQL side effects in `[__migrator]` tables (exact names in SQL files)
- Downstream: none (leaf for persistence); upstream: `engine`, `apply`

## Assumptions and constraints

- Assumption: migrator schema matches embedded DDL in `EnsureTables`.
- Constraint: batch insert path uses `insert_history_openjson.sql` embed (see `subscriber.go`).

## Nominal flow

1. Subscriber `Bootstrap` / `boot` ensures tables once per subscriber lifetime.
2. Loader functions read history for planning.
3. On apply events, subscriber writes batched history rows.

## Off-nominal behavior and failure containment

- Bootstrap errors are surfaced via `BootstrapChecker` to engine (`SetBootstrapChecker` in `app.go`).
- Subscriber notifies optional `ErrorNotifier` on unexpected payloads (warn path).

## Verification and validation

- `make check`
- Integration tests touching audit when run with SQL Server

## Operations and recovery

- Schema migrations for `[__migrator]` require coordinated changes to embed SQL, Go scanners, and tests.

## Open issues and non-goals

- Non-goals: audit does not compute diffs.

## References

- `internal/bus/payload.go`
- `docs/specs/internals/module-bus.md`
