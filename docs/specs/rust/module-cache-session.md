# Module `session`

Lifecycle: `Current`.

## Purpose

Describe the optional `rmigd` Unix-socket proxy that keeps one authenticated
TDS connection warm between CLI invocations.

## Scope

- `crates/core/src/session/` - client, proxy protocol, daemon, socket limits
- `crates/rmigd/` - daemon binary
- `crates/core/src/driver/` - direct and proxied `TimingConn`

The repository has no filesystem plan cache and no process-global plan
snapshot.

## System Context

`rmig` connects directly unless `RMIG_SESSION` names a daemon socket. Both
paths call the same plan and apply code. SQL Server remains the source of live
catalog and audit state.

## Interfaces And Boundaries

- Inputs: `RMIG_SESSION`, `RMIG_SESSION_TOKEN`, SQL endpoint settings.
- Outputs: authenticated request/response frames and TDS query results.
- Runtime boundary: the daemon owns one warm SQL connection and a bounded set
  of local client handlers.
- Limits: `MAX_SESSION_LINE_BYTES`, `MAX_DAEMON_CLIENTS`.

## Assumptions And Constraints

- The daemon requires Unix domain sockets.
- One warm TDS session serializes SQL RPCs.
- A timed-out or transaction-uncertain connection cannot return to reuse.
- Multi-database commands connect directly per database.

## Nominal Flow

1. The CLI tries the configured daemon socket.
2. Client and daemon authenticate with `RMIG_SESSION_TOKEN`.
3. The daemon executes the request on its TDS session.
4. A healthy session remains warm for the next request.

## Off-Nominal Behavior And Failure Containment

- Missing daemon socket: the client logs the condition and connects directly.
- Invalid token or oversized frame: the daemon rejects the request.
- Query timeout or uncertain transaction state: the daemon drops the SQL
  connection before accepting another request.

## Verification And Validation

- `cargo test -p migrator-core --all-features --lib --tests`
- `cargo test -p migrator-core --test rmigd_timeout_recovery_test`
- `make slo`
- `make e2e-all`
- Exit criterion: direct fallback, authentication, timeout cleanup, and E2E
  scenarios pass.

## Operations And Recovery

- Start `rmigd` before exporting `RMIG_SESSION`.
- Remove a stale owned socket only after confirming no daemon process owns it.
- A missing socket needs no manual recovery because the CLI falls back direct.

## Open Issues And Non-Goals

- Open issues: none.
- Non-goals: filesystem plan caching, process-global snapshots, connection
  pooling, and cross-host session sharing.

## References

- `docs/prod-gate.md`
- `ops/perf/README.md`
- `docs/specs/rust/module-db.md`
- `adr/0018-l1-plan-cache.md`
