# ADR-0009: rmigd metrics/health over the socket, not HTTP/Prometheus

Status: Accepted
Date: 2026-07-21

## Context

`rmigd` is a long-running daemon. Audit flagged: no metrics/health surface —
liveness only probeable by socket connect, tracing to stderr the only telemetry.
Operators want quantitative telemetry (requests, reconnects, queue waits) and a
readiness signal (is the warm TDS session connected).

## Decision

Expose metrics over the existing line-framed JSON socket protocol, not a new HTTP
server or Prometheus exporter. New request `{"op":"stats"}` returns
`Response.stats` = JSON `{uptime_s, requests, reconnects, queue_waits,
warm_connection}` from a process-global counter module
(`crates/core/src/session/daemon/metrics.rs`). Handled at the daemon level; it
touches no SQL Server, so it doubles as a liveness/readiness probe. Reconnects and
>100 ms warm-session queue waits also log via `tracing` on stderr.

`Response` gains a `stats` field (`serde default` + `skip_serializing_if`), so the
protocol stays backward-compatible with existing clients.

Rejected: HTTP/Prometheus exporter. `rmigd` runs on a private `0600` local Unix
socket for a CI pipeline, not as a scraped network service. An HTTP server adds a
network listener + dependency + attack surface for no scrape target.

## Consequences

- Metrics are pull (socket `Stats`) and push (tracing on events) — both over
  channels the daemon already owns; CI captures the daemon stderr.
- Counters are global atomics (daemon is single-instance) — no threading through
  signatures. `warm_connection` uses a non-blocking `try_lock` peek.
- Covered by `crates/core/tests/rmigd_stats_test.rs` (live endpoint) + a
  metrics-format unit test. Commit `50d51c0`.
- Not a full observability stack: no histograms, no OTel. Adequate for the role;
  revisit if `rmigd` ever becomes a scraped service.
