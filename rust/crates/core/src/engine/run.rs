use std::sync::{Arc, Mutex};
use std::time::Instant;

use crate::config::{discover_catalog_databases, ensure_catalog_databases_exist, Config};
use crate::domain::Workspace;
use crate::driver::{connect, DbClient, TimingConn};
use crate::error::{Error, Result};
use crate::export::MigrationPlan;
use crate::session;
use crate::timings::{self, PhaseTimings};

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Command {
    Plan,
    Migrate,
    Validate,
    Baseline,
    RepairChecksum,
    Version,
}

pub struct RunOutput {
    pub exit_code: i32,
    pub timings: PhaseTimings,
    pub plan: Option<MigrationPlan>,
}

pub async fn run_command(cmd: Command, cfg: &Config) -> Result<RunOutput> {
    if cmd == Command::Version {
        return Ok(RunOutput {
            exit_code: 0,
            timings: PhaseTimings::default(),
            plan: None,
        });
    }

    let cli_start = Instant::now();
    let databases = discover_catalog_databases(&cfg.sql_root)?;
    ensure_catalog_databases_exist(cfg, &databases).await?;

    let t_scan = Instant::now();
    let mut ws_full = Workspace::default();
    let scan_ms = crate::scan::populate(&mut ws_full, &cfg.sql_root, cfg.skip_git).await?;
    let scan_elapsed = t_scan.elapsed();

    let mut merged = PhaseTimings::default();
    merged.scan_ms = scan_ms;
    let mut exit_code = 0i32;
    let mut last_plan: Option<MigrationPlan> = None;
    let multi = databases.len() > 1;

    for db in &databases {
        let mut cfg_db = cfg.clone();
        cfg_db.database = db.clone();
        let ws = ws_full.for_catalog_database(db);
        if ws.object_count() == 0 && multi {
            continue;
        }
        let out = run_command_for_database(cmd, &cfg_db, ws, scan_elapsed, cli_start, multi).await?;
        exit_code = exit_code.max(out.exit_code);
        merge_timings(&mut merged, &out.timings);
        if out.plan.is_some() {
            last_plan = out.plan;
        }
    }

    merged.plan_wall_ms = timings::dur_ms(cli_start.elapsed());
    merged.engine_ms = merged.plan_wall_ms.saturating_sub(merged.connect_ms);
    merged.cli_wall_ms = merged.plan_wall_ms;

    Ok(RunOutput {
        exit_code,
        timings: merged,
        plan: last_plan,
    })
}

async fn run_command_for_database(
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
    let db_client = if !cfg.session_socket.is_empty() && !multi_db {
        session::connect_daemon(&cfg.session_socket).await?
    } else {
        DbClient::Direct(connect(cfg).await?.client)
    };
    let connect_ms = timings::dur_ms(t_conn.elapsed());
    timings.connect_ms = connect_ms;
    let io_arc = Arc::new(Mutex::new(crate::driver::IoProfile::default()));
    let mut conn = TimingConn::new(db_client, io_arc.clone(), connect_ms);

    let db = crate::db::run_plan_db_phase(cfg, &mut conn, &ws).await?;
    super::warm_store::store_plan_db_snapshot(&ws.layout_digest, &db.checksums, &db.catalog);
    timings.ensure_ms = db.ensure_ms;
    timings.checksums_ms = db.checksums_ms;
    timings.inspect_ms = db.inspect_ms;
    timings.parallel_wall_ms = if db.parallel_wall_ms > 0 {
        db.parallel_wall_ms
    } else {
        db.ensure_ms.max(db.checksums_ms + db.inspect_ms)
    };
    timings.l1_cache_hit = db.l1_hit;
    timings.plan_db_path = db.trace.path_label().to_string();
    timings.plan_db_query_calls = db.trace.query_calls;
    timings.plan_db_query_ms = db.trace.query_ms;
    timings.plan_db_bootstrap = db.trace.bootstrap;
    timings.plan_db_catalog_queried = db.trace.catalog_queried;
    timings.plan_db_checksums_batch_ms = db.trace.checksums_batch_ms;
    timings.plan_db_catalog_ms = db.trace.catalog_ms;
    timings.finish_audit_ms();

    let (mut plan, diff_ms) = crate::plan::compute_diff(&mut ws, &db.catalog, &db.checksums)?;
    plan.command = command_label(cmd).into();
    plan.planned_at = chrono::Utc::now().to_rfc3339();
    timings.diff_ms = diff_ms;

    let exit_code = match super::apply_run::maybe_apply(cmd, &mut conn, cfg, &ws, &mut plan, &mut timings).await
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

fn merge_timings(dst: &mut PhaseTimings, src: &PhaseTimings) {
    dst.connect_ms = dst.connect_ms.max(src.connect_ms);
    dst.ensure_ms = dst.ensure_ms.saturating_add(src.ensure_ms);
    dst.checksums_ms = dst.checksums_ms.saturating_add(src.checksums_ms);
    dst.inspect_ms = dst.inspect_ms.saturating_add(src.inspect_ms);
    dst.diff_ms = dst.diff_ms.saturating_add(src.diff_ms);
    dst.apply_ms = dst.apply_ms.saturating_add(src.apply_ms);
    dst.parallel_wall_ms = dst.parallel_wall_ms.max(src.parallel_wall_ms);
    dst.plan_db_query_ms = dst.plan_db_query_ms.saturating_add(src.plan_db_query_ms);
    dst.plan_db_query_calls = dst.plan_db_query_calls.saturating_add(src.plan_db_query_calls);
    if !src.l1_cache_hit {
        dst.l1_cache_hit = false;
    }
}

fn command_label(cmd: Command) -> &'static str {
    match cmd {
        Command::Plan => "plan",
        Command::Migrate => "migrate",
        Command::Validate => "validate",
        Command::Baseline => "baseline",
        Command::RepairChecksum => "repair-checksum",
        Command::Version => "version",
    }
}
