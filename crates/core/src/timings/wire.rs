use serde::Deserialize;

use super::phase::PhaseTimings;

#[derive(Deserialize, Default)]
#[serde(default)]
pub(super) struct PhaseTimingsWire {
    connect_ms: i64,
    scan_ms: i64,
    inspect_ms: i64,
    checksums_ms: i64,
    ensure_ms: i64,
    parallel_wall_ms: i64,
    audit_ms: i64,
    diff_ms: i64,
    plan_wall_ms: i64,
    cli_wall_ms: i64,
    engine_ms: i64,
    apply_ms: i64,
    audit_flush_ms: i64,
    plan_db_query_calls: i64,
    plan_db_query_ms: i64,
    plan_db_checksums_batch_ms: i64,
    plan_db_catalog_ms: i64,
    plan_db_catalog_sql_ms: i64,
    plan_db_intern_catalog_ms: i64,
    plan_db_round_trips: i64,
    plan_db_path: String,
    l1_cache_hit: bool,
    plan_db_bootstrap: bool,
    plan_db_catalog_queried: bool,
    plan_db_history_empty: bool,
    plan_db_checksums_skipped: bool,
}

impl PhaseTimingsWire {
    pub(super) fn into_phase_timings(self) -> PhaseTimings {
        let mut t = PhaseTimings {
            connect_ms: self.connect_ms,
            scan_ms: self.scan_ms,
            inspect_ms: self.inspect_ms,
            checksums_ms: self.checksums_ms,
            ensure_ms: self.ensure_ms,
            parallel_wall_ms: self.parallel_wall_ms,
            audit_ms: self.audit_ms,
            diff_ms: self.diff_ms,
            plan_wall_ms: self.plan_wall_ms,
            cli_wall_ms: self.cli_wall_ms,
            engine_ms: self.engine_ms,
            apply_ms: self.apply_ms,
            audit_flush_ms: self.audit_flush_ms,
            plan_db_query_calls: self.plan_db_query_calls,
            plan_db_query_ms: self.plan_db_query_ms,
            plan_db_checksums_batch_ms: self.plan_db_checksums_batch_ms,
            plan_db_catalog_ms: self.plan_db_catalog_ms,
            plan_db_catalog_sql_ms: self.plan_db_catalog_sql_ms,
            plan_db_intern_catalog_ms: self.plan_db_intern_catalog_ms,
            plan_db_round_trips: self.plan_db_round_trips,
            plan_db_path: self.plan_db_path,
            flags: 0,
        };
        t.set_l1_cache_hit(self.l1_cache_hit);
        t.set_plan_db_bootstrap(self.plan_db_bootstrap);
        t.set_plan_db_catalog_queried(self.plan_db_catalog_queried);
        t.set_plan_db_history_empty(self.plan_db_history_empty);
        t.set_plan_db_checksums_skipped(self.plan_db_checksums_skipped);
        t
    }
}
