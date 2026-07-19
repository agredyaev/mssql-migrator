# Modules `cache` and `session`

Lifecycle: `Current`.

## Purpose

Describe the retained **L1 snapshot format** and optional **rmigd session daemon** for warm SQL connections.

## Scope

### cache

- `crates/core/src/cache/l1.rs` - disk-backed L1 keyed by `(db_fingerprint, layout_digest)`
- Used from `db::plan_snapshot` and invalidated on apply / DB reset

### session

- `crates/core/src/session/client.rs` - connect to daemon socket
- `crates/core/src/session/protocol.rs` - RPC framing
- `crates/core/src/session/proxy.rs` - `ProxyClient` implementing TDS proxy
- `crates/core/src/session/daemon/mod.rs`, `daemon_rpc.rs` - `rmigd` (feature `session-daemon`)
- `crates/core/src/session/limits.rs` - socket line and client handler limits
- Crate: `crates/rmigd/`

## System context

Product SLO (`cli_wall_ms` < 150 ms) uses `RMIG_SESSION` plus warm SQL Server query plans. Managed-object plans do not use a zero-query L1 hit because live fingerprints must be refreshed. See `make slo` and `ops/perf/cli_phase.sh`.

## Interfaces and boundaries

- L1: `try_load`, `save`, `invalidate_all`
- Session: `connect_daemon`, `connect_session_or_direct`, `resolve_session_token`, `run_daemon`
- Env: `RMIG_SESSION` (socket path), `RMIG_SESSION_TOKEN` (shared auth token)
- Limits: `MAX_SESSION_LINE_BYTES`, `MAX_DAEMON_CLIENTS`, `MAX_L1_CACHE_BYTES`

## Assumptions and constraints

- Assumption: L1 cache directory (`cfg.l1_cache_dir`, default `.rmig/cache`) is writable.
- Constraint: top-level L1/warm snapshots are eligible only when the workspace has no managed objects. Snapshots are still written for format benchmarks and backward-compatible cleanup.
- Constraint: session daemon requires Unix domain socket (feature `session-daemon`).
- Constraint: `rmigd` keeps one warm TDS session. Concurrent socket handlers are bounded by `MAX_DAEMON_CLIENTS`; SQL RPCs serialize on the shared TDS client. A timed-out TDS session is discarded and reconnected before reuse because a cancelled Tiberius request may leave unread protocol state.

## Nominal flow

1. Plan: a managed workspace runs SQL plan DB and saves a snapshot but does not trust that snapshot as live-state evidence; an empty workspace may load it.
2. CLI with session: engine uses proxy conn for all queries in one process invocation; if the socket is missing or unreachable, it logs a warning and falls back to direct TDS for that run.
3. Daemon: `run_daemon` accepts Unix socket clients, waits for a `MAX_DAEMON_CLIENTS` handler slot, then serves the request stream against the single warm TDS session.

## Verification and validation

- `make slo`
- `make check-e2e`
- `crates/core/tests/integration_plan.rs`
- `crates/core/tests/session_fallback_test.rs`
- `crates/core/tests/session_token_test.rs`
- `crates/core/tests/advisory_lock_rmigd_test.rs`
- `crates/core/tests/rmigd_timeout_recovery_test.rs`

## Off-nominal behavior and failure containment

- Failure mode: stale L1 after DB reset without invalidation.
  Containment: managed workspaces bypass it; `db_reset.rs` and successful apply also invalidate it.
- Failure mode: too many local socket clients.
  Containment: `run_daemon` waits for a `MAX_DAEMON_CLIENTS` slot before spawning another handler, so task growth is bounded.
- Failure mode: a daemon SQL request exceeds `RM_COMMAND_TIMEOUT`.
  Containment: return `rmigd: request timed out`, drop the entire TDS connection so SQL Server rolls back its transaction and session lock, then reconnect before the next RPC.
- Failure mode: normal disconnect cleanup cannot roll back or release the session lock.
  Containment: discard the TDS connection instead of silently returning uncertain session state to the shared slot.

## Operations and recovery

- Start `rmigd` before CLI when using `RMIG_SESSION` for the warm path; a missing daemon socket falls back to direct connect automatically.

## Open issues and non-goals

- Non-goals: cross-host L1 sharing.

## References

- `docs/prod-gate.md` - product SLO gate
- `ops/perf/README.md` - Makefile performance and e2e gates
- `docs/specs/rust/module-db.md`
