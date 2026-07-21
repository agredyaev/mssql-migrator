//! Process-global counters for the single daemon instance, exposed over the
//! socket via the `Stats` request and logged on reconnect. This is the daemon's
//! metrics/health surface: it runs on a private local socket, not as a scraped
//! HTTP service, so a lightweight pull-and-log surface fits its role.

use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::OnceLock;
use std::time::Instant;

static REQUESTS: AtomicU64 = AtomicU64::new(0);
static RECONNECTS: AtomicU64 = AtomicU64::new(0);
static QUEUE_WAITS: AtomicU64 = AtomicU64::new(0);
static STARTED: OnceLock<Instant> = OnceLock::new();

/// Marks the daemon start time; call once from `run_daemon`.
pub(super) fn mark_started() {
    let _ = STARTED.get_or_init(Instant::now);
}

/// Records one dispatched client request.
pub(super) fn record_request() {
    REQUESTS.fetch_add(1, Ordering::Relaxed);
}

/// Records one warm-session reconnect.
pub(super) fn record_reconnect() {
    RECONNECTS.fetch_add(1, Ordering::Relaxed);
}

/// Records one head-of-line wait (a client queued for the warm session).
pub(super) fn record_queue_wait() {
    QUEUE_WAITS.fetch_add(1, Ordering::Relaxed);
}

/// Snapshot: `(uptime_seconds, requests, reconnects, queue_waits)`.
pub(super) fn snapshot() -> (u64, u64, u64, u64) {
    let uptime = STARTED.get().map(|s| s.elapsed().as_secs()).unwrap_or(0);
    (
        uptime,
        REQUESTS.load(Ordering::Relaxed),
        RECONNECTS.load(Ordering::Relaxed),
        QUEUE_WAITS.load(Ordering::Relaxed),
    )
}

/// JSON metrics line for the `Stats` response. `warm` reports whether the shared
/// TDS session is currently connected (readiness).
pub(super) fn snapshot_json(warm: bool) -> String {
    let (uptime, requests, reconnects, queue_waits) = snapshot();
    format!(
        "{{\"uptime_s\":{uptime},\"requests\":{requests},\"reconnects\":{reconnects},\
         \"queue_waits\":{queue_waits},\"warm_connection\":{warm}}}"
    )
}

#[cfg(test)]
#[path = "../../tests/daemon_metrics_test.rs"]
mod tests;
