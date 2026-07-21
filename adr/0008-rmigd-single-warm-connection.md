# ADR-0008: rmigd holds one warm TDS connection, no pool

Status: Accepted
Date: 2026-07-21

## Context

`rmigd` exists to skip per-invocation TDS login + catalog warm-up across many
`rmig` calls in a CI pipeline. A `migrate` through the daemon is a sequence of
RPCs (`BEGIN TX → body → history INSERT → COMMIT`) that must run on ONE TDS
session: releasing the session mid-sequence would let another client interleave
in the open transaction and the Session-owned advisory lock.

## Decision

One warm TDS connection behind `Arc<Mutex<Option<RawClient>>>`. A client acquires
the owned mutex guard on first RPC and holds it for the whole client session
(`session/daemon/serve_loop.rs`, `serve::acquire_session`). Concurrent clients
serialize on that guard. Client fan-in bounded by `Semaphore(MAX_DAEMON_CLIENTS)`;
idle clients dropped after 60 s; per-command timeout drops a wedged session and
reconnects lazily (`serve::reconnect`). No connection pool.

## Consequences

- Correctness: transactions and the applock cannot be interleaved by another
  client. Proven by `chaos_kill_mid_apply_test` (concurrent migrates serialize
  exactly once via the applock).
- Throughput ceiling: head-of-line blocking — one slow client blocks others up to
  the idle/command timeout. Accepted for the CI-helper role.
- Empirically validated no cost at real concurrency (`ops/perf/hol_probe.sh`):
  1/2/4/8 concurrent `plan` clients, daemon-serialized vs direct-cold-connect are
  statistically identical (N=8: p95 0.72 s vs 0.73 s), zero >100 ms queue events.
- Tripwire, not a pool: a >100 ms wait for the warm session logs
  `rmigd: queued for warm session` and increments the `queue_waits` metric
  (ADR-0009). Recurring warnings are the signal to revisit with a pool; a pool
  would preserve correctness (applock is Session-owned, per-connection).
