//! Workflow engine + plan DB SLO helpers (git delta migrate).

use migrator_core::config::Config;
use migrator_core::db::{
    max_parallel_wall_ms, plan_db_path_from_label, plan_db_slo_exempt, PlanDbFlags, PlanDbTimings,
    PlanDbTrace,
};
use migrator_core::engine::{run_command, Command, RunOutput};
use migrator_core::error::Result;
use migrator_core::timings::PhaseTimings;

pub async fn migrate(cfg: &Config) -> Result<RunOutput> {
    let mut c = cfg.clone();
    c.skip_git = false;
    c.session_socket.clear();
    run_command(Command::Migrate, &c).await
}

pub fn log_timings(label: &str, t: &PhaseTimings) {
    eprintln!(
        "{label}: wall={}ms conn={} scan={} insp={} par={} apply={} l1={} path={} q={} query_ms={}",
        t.plan_wall_ms,
        t.connect_ms,
        t.scan_ms,
        t.inspect_ms,
        t.parallel_wall_ms,
        t.apply_ms,
        t.l1_cache_hit(),
        t.plan_db_path,
        t.plan_db_query_calls,
        t.plan_db_query_ms
    );
    let trace = PlanDbTrace {
        path: plan_db_path_from_label(&t.plan_db_path),
        timings: PlanDbTimings {
            checksums_batch_ms: t.plan_db_checksums_batch_ms,
            catalog_ms: t.plan_db_catalog_ms,
            catalog_sql_ms: t.plan_db_catalog_sql_ms,
            intern_catalog_ms: t.plan_db_intern_catalog_ms,
            query_calls: t.plan_db_query_calls,
            query_ms: t.plan_db_query_ms,
            round_trips: t.plan_db_round_trips,
            ..PlanDbTimings::default()
        },
        flags: PlanDbFlags {
            bootstrap: t.plan_db_bootstrap(),
            catalog_queried: t.plan_db_catalog_queried(),
            history_empty: t.plan_db_history_empty(),
            checksums_skipped: t.plan_db_checksums_skipped(),
            ..PlanDbFlags::default()
        },
    };
    migrator_core::db::maybe_append_trace(label, &trace, t.parallel_wall_ms);
}

pub fn assert_plan_db_par_slo(label: &str, t: &PhaseTimings) {
    if plan_db_slo_exempt(&t.plan_db_path, t.l1_cache_hit()) {
        return;
    }
    let max = max_parallel_wall_ms();
    assert!(
        t.parallel_wall_ms <= max,
        "{label}: parallel_wall_ms={}ms exceeds SLO {}ms (path={} q={} query_ms={})",
        t.parallel_wall_ms,
        max,
        t.plan_db_path,
        t.plan_db_query_calls,
        t.plan_db_query_ms
    );
}
