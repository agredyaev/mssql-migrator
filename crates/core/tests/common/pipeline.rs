//! Pipeline-level plan run (direct SQL connect, no rmigd).

use migrator_core::config::Config;
use migrator_core::domain::Workspace;
use migrator_core::driver::{connect, DbClient, IoProfile, TimingConn};
use migrator_core::error::Result;
use migrator_core::export::MigrationPlan;
use migrator_core::timings::{self, PhaseTimings};
use std::sync::{Arc, Mutex};
use std::time::Instant;

pub async fn run_plan_pipeline(cfg: &Config) -> Result<(MigrationPlan, PhaseTimings, IoProfile)> {
    let start_all = Instant::now();
    let mut timings = PhaseTimings::default();
    let skip_l1_invalidate = io_debug_skip_l1_invalidate();

    let mut ws = Workspace::default();
    timings.scan_ms = migrator_core::scan::populate(&mut ws, &cfg.sql_root, cfg.skip_git()).await?;

    if !skip_l1_invalidate {
        let fp = format!("{}_{}", cfg.server, cfg.database);
        let l1 = migrator_core::cache::l1::L1Cache::new(&cfg.l1_cache_dir);
        let _ = l1.invalidate_all(&fp);
        let db_fp =
            migrator_core::audit::db_fingerprint(&cfg.server, &cfg.port, &cfg.user, &cfg.database);
        migrator_core::db::invalidate_inspect_cache(&db_fp);
    }

    let t_conn = Instant::now();
    let io_arc = Arc::new(Mutex::new(IoProfile::default()));
    let mut conn = TimingConn::new(
        DbClient::Direct(connect(cfg).await?.client),
        io_arc.clone(),
        timings::dur_ms(t_conn.elapsed()),
    );
    timings.connect_ms = timings::dur_ms(t_conn.elapsed());

    let db = migrator_core::db::run_plan_db_phase(cfg, &mut conn, &ws, false).await?;
    timings.ensure_ms = db.ensure_ms;
    timings.checksums_ms = db.checksums_ms;
    timings.inspect_ms = db.inspect_ms;
    timings.parallel_wall_ms = if db.parallel_wall_ms > 0 {
        db.parallel_wall_ms
    } else {
        db.ensure_ms.max(db.checksums_ms + db.inspect_ms)
    };
    timings.set_l1_cache_hit(db.l1_hit);
    timings.finish_audit_ms();

    let (mut plan, diff_ms) =
        migrator_core::plan::compute_diff(&mut ws, &db.catalog, &db.checksums)?;
    timings.diff_ms = diff_ms;
    if plan.uses_slim_rows() {
        plan.ensure_objects_materialized(&ws);
    }
    timings.plan_wall_ms = timings::dur_ms(start_all.elapsed());
    timings.engine_ms = timings.plan_wall_ms.saturating_sub(timings.connect_ms);

    let io = conn.io_snapshot();
    Ok((plan, timings, io))
}

fn io_debug_skip_l1_invalidate() -> bool {
    if matches!(
        std::env::var("RMIG_IO_DEBUG_SKIP_L1_INVALIDATE").as_deref(),
        Ok("1") | Ok("true") | Ok("yes")
    ) {
        return true;
    }
    if matches!(
        std::env::var("RMIG_E2E_SCENARIO").as_deref(),
        Ok("skip_unchanged_plan") | Ok("catalog_cache_plan")
    ) {
        return true;
    }
    false
}
