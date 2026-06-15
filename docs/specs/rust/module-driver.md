# Module `driver`

Lifecycle: `Current`.

## Purpose

Describe the **TDS SQL Server client layer**: connect, query, exec, row decoding, and I/O timing instrumentation.

## Scope

- `crates/core/src/driver/mssql.rs` - TDS protocol / client
- `crates/core/src/driver/db_client.rs` - direct vs proxy client enum
- `crates/core/src/driver/timing.rs` - `TimingConn`, connect wrapper
- `crates/core/src/driver/io_profile.rs` - query call count and wall ms
- `crates/core/src/driver/row.rs` - row cell accessors

## System context

All SQL I/O from `audit`, `db`, `apply`, `lock`, and `scaffold` flows through `TimingConn`. Session daemon mode uses `DbClient::Proxy` via `session::ProxyClient`.

## Interfaces and boundaries

- Public: `connect`, `TimingConn`, `DbClient`, `IoProfile`
- Inputs: `Config` (server, port, database, credentials)
- Outputs: query result rows, errors as `Error::Sql`
- Must not import `plan` or `apply`
- I/O timing counters are best-effort observability data. If their mutex is poisoned, the driver recovers existing counters instead of panicking.

## Assumptions and constraints

- `trust_server_certificate` supported for Docker dev.
- Integrated Windows auth is outside the normal SQL-auth path; see `docs/operational-contract.md` for runtime environment variables.

## Nominal flow

1. TCP connect + TDS login.
2. Parameterized queries (`@p1`, `@p2`, …).
3. `io_snapshot()` aggregates RT count for plan DB trace.

## Verification and validation

- Integration tests with `RMIG_RUN_SQLSERVER_INTEGRATION=1`
- `make check`

## Off-nominal behavior and failure containment

- Failure mode: connection refused or login failure.
  Containment: error returned to engine; no partial plan state on conn failure.
- Failure mode: I/O profile mutex poison after an internal panic.
  Containment: recover the mutex contents and continue metrics aggregation.

## Operations and recovery

- Verify Docker MSSQL with `make db-up` before integration tests.

## Open issues and non-goals

- Non-goals: connection pooling beyond `rmigd` session (see `module-cache-session.md`).

## References

- `docs/rmig-rust.md`
- `docs/specs/rust/module-cache-session.md`
