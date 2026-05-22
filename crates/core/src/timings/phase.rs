use super::flags::{
    flag_get, flag_set, PHASE_FLAG_L1_CACHE_HIT, PHASE_FLAG_PLAN_DB_BOOTSTRAP,
    PHASE_FLAG_PLAN_DB_CATALOG_QUERIED, PHASE_FLAG_PLAN_DB_CHECKSUMS_SKIPPED,
    PHASE_FLAG_PLAN_DB_HISTORY_EMPTY,
};

/// Wall durations for plan/CLI profiling (milliseconds).
#[derive(Debug, Default, Clone, PartialEq, Eq)]
pub struct PhaseTimings {
    pub connect_ms: i64,
    pub scan_ms: i64,
    pub inspect_ms: i64,
    pub checksums_ms: i64,
    pub ensure_ms: i64,
    pub parallel_wall_ms: i64,
    pub audit_ms: i64,
    pub diff_ms: i64,
    pub plan_wall_ms: i64,
    pub cli_wall_ms: i64,
    pub engine_ms: i64,
    pub apply_ms: i64,
    pub audit_flush_ms: i64,
    pub plan_db_query_calls: i64,
    pub plan_db_query_ms: i64,
    pub plan_db_checksums_batch_ms: i64,
    pub plan_db_catalog_ms: i64,
    pub plan_db_catalog_sql_ms: i64,
    pub plan_db_intern_catalog_ms: i64,
    pub plan_db_round_trips: i64,
    pub plan_db_path: String,
    pub flags: u8,
}

impl PhaseTimings {
    pub fn finish_audit_ms(&mut self) {
        self.audit_ms = self.ensure_ms + self.checksums_ms;
    }

    #[inline]
    pub fn l1_cache_hit(&self) -> bool {
        flag_get(self.flags, PHASE_FLAG_L1_CACHE_HIT)
    }

    #[inline]
    pub fn set_l1_cache_hit(&mut self, on: bool) {
        flag_set(&mut self.flags, PHASE_FLAG_L1_CACHE_HIT, on);
    }

    #[inline]
    pub fn plan_db_bootstrap(&self) -> bool {
        flag_get(self.flags, PHASE_FLAG_PLAN_DB_BOOTSTRAP)
    }

    #[inline]
    pub fn set_plan_db_bootstrap(&mut self, on: bool) {
        flag_set(&mut self.flags, PHASE_FLAG_PLAN_DB_BOOTSTRAP, on);
    }

    #[inline]
    pub fn plan_db_catalog_queried(&self) -> bool {
        flag_get(self.flags, PHASE_FLAG_PLAN_DB_CATALOG_QUERIED)
    }

    #[inline]
    pub fn set_plan_db_catalog_queried(&mut self, on: bool) {
        flag_set(&mut self.flags, PHASE_FLAG_PLAN_DB_CATALOG_QUERIED, on);
    }

    #[inline]
    pub fn plan_db_history_empty(&self) -> bool {
        flag_get(self.flags, PHASE_FLAG_PLAN_DB_HISTORY_EMPTY)
    }

    #[inline]
    pub fn set_plan_db_history_empty(&mut self, on: bool) {
        flag_set(&mut self.flags, PHASE_FLAG_PLAN_DB_HISTORY_EMPTY, on);
    }

    #[inline]
    pub fn plan_db_checksums_skipped(&self) -> bool {
        flag_get(self.flags, PHASE_FLAG_PLAN_DB_CHECKSUMS_SKIPPED)
    }

    #[inline]
    pub fn set_plan_db_checksums_skipped(&mut self, on: bool) {
        flag_set(&mut self.flags, PHASE_FLAG_PLAN_DB_CHECKSUMS_SKIPPED, on);
    }
}
