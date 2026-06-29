use std::sync::{Arc, Mutex};
use std::time::Instant;

use crate::config::Config;
use crate::domain::Workspace;
use crate::driver::{connect, DbClient, TimingConn};
use crate::error::{Error, Result};
use crate::timings::{self, PhaseTimings};

use super::planned_at::resolved_planned_at;
use super::types::{command_label, Command, RunOutput};

pub(super) async fn run_command_for_database(
    cmd: Command,
    cfg: &Config,
    mut ws: Workspace,
    scan_elapsed: std::time::Duration,
    cli_start: Instant,
    multi_db: bool,
) -> Result<RunOutput> {
    let mut timings = PhaseTimings::default();
    timings.scan_ms = timings::dur_ms(scan_elapsed);

    let t_conn = Instant::now();
    let db_client = if multi_db {
        DbClient::Direct(connect(cfg).await?.client)
    } else {
        crate::session::connect_session_or_direct(cfg).await?
    };
    let connect_ms = timings::dur_ms(t_conn.elapsed());
    timings.connect_ms = connect_ms;
    let io_arc = Arc::new(Mutex::new(crate::driver::IoProfile::default()));
    let mut conn = TimingConn::new(db_client, io_arc.clone(), connect_ms);
    conn.set_command_timeout(cfg.command_timeout);

    let db = crate::db::run_plan_db_phase(cfg, &mut conn, &ws).await?;
    let server_database = format!("{}_{}", cfg.server, cfg.database);
    super::super::warm_store::store_plan_db_snapshot(
        &server_database,
        &ws.layout_digest,
        &db.checksums,
        &db.catalog,
    );
    timings.ensure_ms = db.ensure_ms;
    timings.checksums_ms = db.checksums_ms;
    timings.inspect_ms = db.inspect_ms;
    timings.parallel_wall_ms = if db.parallel_wall_ms > 0 {
        db.parallel_wall_ms
    } else {
        db.ensure_ms.max(db.checksums_ms + db.inspect_ms)
    };
    timings.set_l1_cache_hit(db.l1_hit);
    timings.plan_db_path = db.trace.path_label().to_string();
    timings.plan_db_query_calls = db.trace.timings.query_calls;
    timings.plan_db_query_ms = db.trace.timings.query_ms;
    timings.set_plan_db_bootstrap(db.trace.flags.bootstrap);
    timings.set_plan_db_catalog_queried(db.trace.flags.catalog_queried);
    timings.plan_db_checksums_batch_ms = db.trace.timings.checksums_batch_ms;
    timings.plan_db_catalog_ms = db.trace.timings.catalog_ms;
    timings.plan_db_catalog_sql_ms = db.trace.timings.catalog_sql_ms;
    timings.plan_db_intern_catalog_ms = db.trace.timings.intern_catalog_ms;
    timings.set_plan_db_history_empty(db.trace.flags.history_empty);
    timings.set_plan_db_checksums_skipped(db.trace.flags.checksums_skipped);
    timings.plan_db_round_trips = db.trace.timings.round_trips;
    timings.finish_audit_ms();

    let (mut plan, diff_ms) = crate::plan::compute_diff(&mut ws, &db.catalog, &db.checksums)?;
    plan.command = command_label(cmd).into();
    plan.planned_at = resolved_planned_at();
    timings.diff_ms = diff_ms;
    if plan.uses_slim_rows() {
        plan.ensure_objects_materialized(&ws);
    }

    let exit_code = match super::super::apply_run::maybe_apply(
        cmd,
        &mut conn,
        cfg,
        &ws,
        &mut plan,
        &mut timings,
    )
    .await
    {
        Err(Error::PlanBlocked) if cmd == Command::Migrate => crate::error::EXIT_PLAN_BLOCKED,
        Err(e) => return Err(e),
        Ok(()) => 0,
    };

    timings.plan_wall_ms = timings::dur_ms(cli_start.elapsed());
    timings.engine_ms = timings.plan_wall_ms.saturating_sub(timings.connect_ms);
    timings.cli_wall_ms = timings.plan_wall_ms;

    Ok(RunOutput {
        exit_code,
        timings,
        plan: Some(plan),
    })
}
