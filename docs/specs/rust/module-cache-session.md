# Technical Document: Modules `cache` and `session`

Lifecycle: `Current`.

## Purpose

Describe **process-local L1 plan cache** and optional **rmigd session daemon** for warm SQL connections.

## Scope

### cache

- `rust/crates/core/src/cache/l1.rs` — disk-backed L1 keyed by `(db_fingerprint, layout_digest)`
- Used from `db::plan_snapshot` and invalidated on apply / DB reset

### session

- `rust/crates/core/src/session/client.rs` — connect to daemon socket
- `rust/crates/core/src/session/protocol.rs` — RPC framing
- `rust/crates/core/src/session/proxy.rs` — `ProxyClient` implementing TDS proxy
- `rust/crates/core/src/session/daemon.rs`, `daemon_rpc.rs` — `rmigd` (feature `session-daemon`)
- Crate: `rust/crates/rmigd/`

## System context

Product SLO (`cli_wall_ms` < 150 ms) assumes warm path: L1 hit and/or `RMIG_SESSION` to `rmigd` avoiding connect + cold catalog. See `make rust-slo` and `ops/perf/rust_cli_phase.sh`.

## Interfaces and boundaries

- L1: `try_load`, `save`, `invalidate_all`
- Session: `connect_daemon`, `run_daemon`
- Env: `RMIG_SESSION` (socket path), `RMIG_L1_CACHE_DIR`

## Assumptions and constraints

- Assumption: L1 cache directory is writable (`RMIG_L1_CACHE_DIR` or default under temp).
- Constraint: session daemon requires Unix domain socket (feature `session-daemon`).

## Nominal flow

1. Plan: L1 try_load → on miss, SQL plan DB → L1 save.
2. CLI with session: engine uses proxy conn for all queries in one process invocation.

## Verification and validation

- `make rust-slo`
- `rust/crates/core/tests/integration_plan.rs`

## Off-nominal behavior and failure containment

- Failure mode: stale L1 after DB reset without invalidation.
  Containment: `db_reset.rs` calls `l1.invalidate_all`; apply also invalidates on success.

## Operations and recovery

- Start `rmigd` before CLI when using `RMIG_SESSION`; stop daemon to force cold connect.

## Open issues and non-goals

- Non-goals: cross-host L1 sharing.

## References

- `docs/rust-port-plan.md` — Product SLO
- `docs/specs/rust/module-db.md`
