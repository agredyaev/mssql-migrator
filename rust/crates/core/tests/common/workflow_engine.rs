//! Workflow engine + plan DB SLO helpers (git delta migrate).

use migrator_core::config::Config;
use migrator_core::db::{max_parallel_wall_ms, PlanDbTrace};
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
        t.l1_cache_hit,
        t.plan_db_path,
        t.plan_db_query_calls,
        t.plan_db_query_ms
    );
    let trace = PlanDbTrace {
        path: None,
        cache_load_ms: 0,
        checksums_batch_ms: t.plan_db_checksums_batch_ms,
        catalog_ms: t.plan_db_catalog_ms,
        query_calls: t.plan_db_query_calls,
        query_ms: t.plan_db_query_ms,
        bootstrap: t.plan_db_bootstrap,
        scoped_hit: false,
        catalog_queried: t.plan_db_catalog_queried,
    };
    migrator_core::db::maybe_append_trace(label, &trace, t.parallel_wall_ms);
}

pub fn assert_plan_db_par_slo(label: &str, t: &PhaseTimings) {
    if t.l1_cache_hit {
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
