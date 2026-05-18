# Technical Document: Modules `internal/driver` and `internal/driver/mssql`

Lifecycle: `Current`.

## Purpose

Define the **database portability boundary**: minimal `driver.Conn` surface used by `db`, `audit`, `apply`, and `lock`, plus the **Microsoft SQL Server** implementation.

## Scope

- `internal/driver/conn.go` — `Conn`, `Rows`, `Result`, `DefaultMaxParameters`
- `internal/driver/mssql/*.go` — `Open`, connection pooling hooks, mapping to `database/sql` / `go-mssqldb`
- `internal/driver/conn_key.go` and tests — connection-scoped helpers if present

## System context

`cmd/rmig` passes `connect` into `app.Run`; production uses `mssql.Open`. Tests swap in mocks implementing `driver.Conn`.

## Interfaces and boundaries

- `Conn` methods: `QueryContext`, `QueryStringsContext`, `QueryStringSlicesContext`, `ExecContext`, `Ping`, `Close`
- Callers must not depend on driver-specific types outside `internal/driver/mssql`

## Assumptions and constraints

- Assumption: string-only query helpers remain available for hot paths that avoid `[]any` boxing until the MSSQL layer.
- Constraint: new query patterns should extend `Conn` only when multiple callers need the same abstraction.

## Nominal flow

1. `Open` validates config and returns `Conn`.
2. Callers use context-aware methods for every query/exec.

## Off-nominal behavior and failure containment

- Connection errors propagate as Go errors; `app.Run` maps them to exit codes.

## Verification and validation

- `make check`
- `make test-int` exercises MSSQL-backed paths in `internal/app`

## Operations and recovery

- TLS / trust flags originate from `types.Config` fields populated in `internal/app/config.go`.

## Open issues and non-goals

- Non-goals: this repository currently ships one concrete driver (`mssql`); other engines would add new packages implementing `Conn`.

## References

- `internal/types/config.go`
- `cmd/rmig/main.go`
