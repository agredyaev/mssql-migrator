//! Run-time execution metrics compilation and phase-timing recorders.
//!
//! ### Purpose
//! Accurately records individual timing durations and metadata flags (like L1 cache hits, plan DB bootstraps)
//! for every major execution phase of `rmig` to support wall time SLO audits.
//!
//! ### Architectural Context
//! - **Inputs**: Durations of execution segments.
//! - **Outputs**: Hydrated `PhaseTimings` structs.
//! - **Boundaries**: Operates in memory as an i64-millisecond accumulator during active commands; no I/O, no allocations per timing event.
//!
//! ### Nominal Flow
//! 1. Measure duration of a phase (e.g. workspace scans, diff calculations).
//! 2. Append durations and flag parameters (`PhaseTimings`).
//! 3. Convert metrics to standard JSON formatting for stdout write reports.
//!
//! ### Off-Nominal & Failure Containment
//! - **Timings Overflow**: Keeps metrics in 64-bit integer values to eliminate integer overflows during long-running migrations.

mod phase;

pub use phase::PhaseTimings;

/// Converts `d` to whole milliseconds as an `i64`.
pub fn dur_ms(d: std::time::Duration) -> i64 {
    d.as_millis() as i64
}
