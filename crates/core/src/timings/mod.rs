//! Run-time execution metrics compilation and phase-timing recorders.
//!
//! ### Purpose
//! Accurately records individual timing durations and metadata flags (like L1 cache hits, plan DB bootstraps)
//! for every major execution phase of `rmig` to support wall time SLO audits.
//!
//! ### Architectural Context
//! - **Inputs**: Durations of execution segments.
//! - **Outputs**: Hydrated `PhaseTimings` structs.
//! - **Boundaries**: Operates in memory as a simple accumulator during active commands.
//!
//! ### Nominal Flow
//! 1. Measure duration of a phase (e.g. workspace scans, diff calculations).
//! 2. Append durations and flag parameters (`PhaseTimings`).
//! 3. Convert metrics to standard JSON formatting for stdout write reports.
//!
//! ### Off-Nominal & Failure Containment
//! - **Timings Overflow**: Keeps metrics in 64-bit integer values to eliminate integer overflows during long-running migrations.

mod flags;
mod phase;
mod serde_impl;
mod wire;

pub use flags::{
    PHASE_FLAG_L1_CACHE_HIT, PHASE_FLAG_PLAN_DB_BOOTSTRAP, PHASE_FLAG_PLAN_DB_CATALOG_QUERIED,
    PHASE_FLAG_PLAN_DB_CHECKSUMS_SKIPPED, PHASE_FLAG_PLAN_DB_HISTORY_EMPTY,
};
pub use phase::PhaseTimings;

pub fn dur_ms(d: std::time::Duration) -> i64 {
    d.as_millis() as i64
}
