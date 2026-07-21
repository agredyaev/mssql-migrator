# ADR-0012: Graceful shutdown drops the run future; server rolls back

Status: Accepted
Date: 2026-07-21

## Context

A `rmig` run or the `rmigd` daemon may receive SIGINT/SIGTERM mid-work (CI
cancel, supervisor stop). Must not leave a half-applied object committed or an
orphaned open transaction / held advisory lock.

## Decision

Both processes race the work future against a signal listener with
`tokio::select!`. On signal, drop the work future. Dropping closes the TDS
connection; SQL Server rolls back any open transaction and releases the
Session-owned advisory lock on disconnect. No explicit in-flight rollback code.

- CLI (`crates/cli/src/signals.rs`, `main.rs`): SIGINT/SIGTERM → drop run future,
  write a failure report, exit `130`.
- Daemon (`crates/rmigd/src/shutdown.rs`, `main.rs`): SIGINT/SIGTERM → drop
  `run_daemon` future (closes listener + warm TDS conn), best-effort unlink the
  socket, exit `0` (clean stop for supervisors). Added because the daemon
  previously looped forever on `accept` with no signal branch.

## Consequences

- Because body+history commit atomically inside one transaction (ADR-0002), a
  dropped run leaves each object fully applied+recorded or untouched. Rerun
  converges (skip-unchanged).
- Verified: daemon `kill -TERM` → exit 0 + socket removed (audit soak tail);
  documented contract in `docs/ci-usage.md`.
- Residual window: a COMMIT already sent to the server may complete server-side
  while the client exits `130`. The audit-history model makes the rerun converge,
  so no divergence — but it is best-effort, not a two-phase guarantee.
- The daemon does not drain in-flight clients on shutdown (drop is immediate);
  acceptable — each client's transaction rolls back cleanly on disconnect.
