# Modules `cache` and `session`

Lifecycle: `Current`.

## Purpose

Describe **process-local L1 plan cache** and optional **rmigd session daemon** for warm SQL connections.

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

Product SLO (`cli_wall_ms` < 150 ms) assumes warm path: L1 hit and/or `RMIG_SESSION` to `rmigd` avoiding connect + cold catalog. See `make slo` and `ops/perf/cli_phase.sh`.

## Interfaces and boundaries

- L1: `try_load`, `save`, `invalidate_all`
- Session: `connect_daemon`, `connect_session_or_direct`, `resolve_session_token`, `run_daemon`
- Env: `RMIG_SESSION` (socket path), `RMIG_L1_CACHE_DIR`
- Limits: `MAX_SESSION_LINE_BYTES`, `MAX_DAEMON_CLIENTS`, `MAX_L1_CACHE_BYTES`

## Assumptions and constraints

- Assumption: L1 cache directory is writable (`RMIG_L1_CACHE_DIR` or default under temp).
- Constraint: session daemon requires Unix domain socket (feature `session-daemon`).
- Constraint: `rmigd` keeps one warm TDS session. Concurrent socket handlers are bounded by `MAX_DAEMON_CLIENTS`; SQL RPCs serialize on the shared TDS client.

## Nominal flow

1. Plan: L1 try_load → on miss, SQL plan DB → L1 save.
2. CLI with session: engine uses proxy conn for all queries in one process invocation; if the socket is missing or unreachable, it logs a warning and falls back to direct TDS for that run.
3. Daemon: `run_daemon` accepts Unix socket clients, waits for a `MAX_DAEMON_CLIENTS` handler slot, then serves the request stream against the single warm TDS session.

## Verification and validation

- `make slo`
- `make check-e2e`
- `crates/core/tests/integration_plan.rs`
- `crates/core/tests/session_fallback_test.rs`
- `crates/core/tests/session_token_test.rs`
- `crates/core/tests/advisory_lock_rmigd_test.rs`

## Off-nominal behavior and failure containment

- Failure mode: stale L1 after DB reset without invalidation.
  Containment: `db_reset.rs` calls `l1.invalidate_all`; apply also invalidates on success.
- Failure mode: too many local socket clients.
  Containment: `run_daemon` waits for a `MAX_DAEMON_CLIENTS` slot before spawning another handler, so task growth is bounded.

## Operations and recovery

- Start `rmigd` before CLI when using `RMIG_SESSION` for the warm path; a missing daemon socket falls back to direct connect automatically.

## Open issues and non-goals

- Non-goals: cross-host L1 sharing.

## References

- `docs/prod-gate.md` - product SLO gate
- `ops/perf/README.md` - Makefile performance and e2e gates
- `docs/specs/rust/module-db.md`
