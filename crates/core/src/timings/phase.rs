//! [`PhaseTimings`] — flat struct of per-phase wall durations in milliseconds.
//!
//! ### Purpose
//! Accumulates wall-clock measurements for every `rmig` execution phase so
//! SLO audits and JSON reporting can inspect where time is spent.
//!
//! ### Flag bit-fields
//! The `flags` field encodes boolean metadata: L1 cache hit, plan-DB bootstrap,
//! catalog queried, history empty, checksums skipped — accessed via getter/setter
//! methods.

use super::flags::{
    flag_get, flag_set, PHASE_FLAG_L1_CACHE_HIT, PHASE_FLAG_PLAN_DB_BOOTSTRAP,
    PHASE_FLAG_PLAN_DB_CATALOG_QUERIED, PHASE_FLAG_PLAN_DB_CHECKSUMS_SKIPPED,
    PHASE_FLAG_PLAN_DB_HISTORY_EMPTY,
};

/// Wall durations for plan/CLI profiling (milliseconds).
#[derive(Debug, Default, Clone, PartialEq, Eq)]
pub struct PhaseTimings {
    /// Time to establish a TDS connection.
    pub connect_ms: i64,
    /// Time to scan the SQL tree on disk.
    pub scan_ms: i64,
    /// Time to inspect the database catalog.
    pub inspect_ms: i64,
    /// Time to load or compute checksums.
    pub checksums_ms: i64,
    /// Time to ensure plan-DB schema and tables.
    pub ensure_ms: i64,
    /// Total parallel wall time across plan-DB queries.
    pub parallel_wall_ms: i64,
    /// Time for audit-history operations.
    pub audit_ms: i64,
    /// Time to compute the diff (plan vs DB state).
    pub diff_ms: i64,
    /// Total wall time for the plan phase end-to-end.
    pub plan_wall_ms: i64,
    /// Wall time for the CLI driver (full entry-point).
    pub cli_wall_ms: i64,
    /// Time spent in the engine run loop.
    pub engine_ms: i64,
    /// Time to apply changes to the database.
    pub apply_ms: i64,
    /// Time to flush audit history after apply.
    pub audit_flush_ms: i64,
    /// Number of plan-DB query calls made.
    pub plan_db_query_calls: i64,
    /// Cumulative time for plan-DB query round-trips.
    pub plan_db_query_ms: i64,
    /// Time for plan-DB checksums batch load.
    pub plan_db_checksums_batch_ms: i64,
    /// Time for plan-DB catalog queries.
    pub plan_db_catalog_ms: i64,
    /// Time for plan-DB catalog SQL execution.
    pub plan_db_catalog_sql_ms: i64,
    /// Time to intern catalog strings into the domain arena.
    pub plan_db_intern_catalog_ms: i64,
    /// Number of TDS round-trips made during plan-DB.
    pub plan_db_round_trips: i64,
    /// Path to the plan-DB trace file.
    pub plan_db_path: String,
    /// Bit-field encoding boolean phase flags (L1 hit, bootstrap, …).
    pub flags: u8,
}

impl PhaseTimings {
    /// Sets `audit_ms` to the sum of `ensure_ms + checksums_ms`.
    pub fn finish_audit_ms(&mut self) {
        self.audit_ms = self.ensure_ms + self.checksums_ms;
    }

    /// True when the L1 filesystem cache was used for plan data.
    #[inline]
    pub fn l1_cache_hit(&self) -> bool {
        flag_get(self.flags, PHASE_FLAG_L1_CACHE_HIT)
    }

    /// Set L1 cache hit flag.
    #[inline]
    pub fn set_l1_cache_hit(&mut self, on: bool) {
        flag_set(&mut self.flags, PHASE_FLAG_L1_CACHE_HIT, on);
    }

    /// True when the plan-DB was bootstrapped (tables created) in this run.
    #[inline]
    pub fn plan_db_bootstrap(&self) -> bool {
        flag_get(self.flags, PHASE_FLAG_PLAN_DB_BOOTSTRAP)
    }

    /// Set plan-DB bootstrap flag.
    #[inline]
    pub fn set_plan_db_bootstrap(&mut self, on: bool) {
        flag_set(&mut self.flags, PHASE_FLAG_PLAN_DB_BOOTSTRAP, on);
    }

    /// True when the plan-DB catalog was queried (not fully cached).
    #[inline]
    pub fn plan_db_catalog_queried(&self) -> bool {
        flag_get(self.flags, PHASE_FLAG_PLAN_DB_CATALOG_QUERIED)
    }

    /// Set catalog-queried flag.
    #[inline]
    pub fn set_plan_db_catalog_queried(&mut self, on: bool) {
        flag_set(&mut self.flags, PHASE_FLAG_PLAN_DB_CATALOG_QUERIED, on);
    }

    /// True when the audit-history table was empty at plan time.
    #[inline]
    pub fn plan_db_history_empty(&self) -> bool {
        flag_get(self.flags, PHASE_FLAG_PLAN_DB_HISTORY_EMPTY)
    }

    /// Set history-empty flag.
    #[inline]
    pub fn set_plan_db_history_empty(&mut self, on: bool) {
        flag_set(&mut self.flags, PHASE_FLAG_PLAN_DB_HISTORY_EMPTY, on);
    }

    /// True when checksum loading was skipped (no audit tables).
    #[inline]
    pub fn plan_db_checksums_skipped(&self) -> bool {
        flag_get(self.flags, PHASE_FLAG_PLAN_DB_CHECKSUMS_SKIPPED)
    }

    /// Set checksums-skipped flag.
    #[inline]
    pub fn set_plan_db_checksums_skipped(&mut self, on: bool) {
        flag_set(&mut self.flags, PHASE_FLAG_PLAN_DB_CHECKSUMS_SKIPPED, on);
    }
}
