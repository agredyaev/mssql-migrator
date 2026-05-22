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
