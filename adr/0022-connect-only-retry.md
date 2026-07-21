# ADR-0022: Connect-phase-only retry, no statement-level retry

Status: Accepted
Date: 2026-07-21

## Context

Transient failures happen: a TCP/TLS handshake blip, a deadlock, a lock timeout.
Retrying is tempting. But retrying a DDL statement or a transaction body risks
applying it twice — the migrator mutates schema and writes audit history, so a
duplicate apply corrupts state.

## Decision

Retry ONLY the connection phase; never a statement, transaction, or command.
`crates/core/src/driver/mssql.rs::connect`: `MAX_ATTEMPTS = 3`, linear backoff
`500ms * attempt`, retried only on `Error::Conn` (transport/handshake). SQL and
config errors never retried. Connect steps are timeout-bounded.

No statement-level retry. A transient deadlock or lock-timeout surfaces as an
error (exit 5 / 7); the advisory-lock deadlock-victim (`-3`) is surfaced, not
retried (ADR-0016). Re-running the whole command is safe (idempotent, ADR-0002),
so recovery is "rerun the run," not "retry the statement."

## Consequences

- No path can apply a DDL statement or write a history row twice → no duplicate
  application, no at-least-once hazard on the mutating path.
- A transient deadlock fails the run; the operator (or CI) reruns, which converges
  via skip-unchanged.
- Connection blips (the common transient) are absorbed silently within the
  command.
