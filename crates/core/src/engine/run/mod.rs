mod database;
mod merge;
mod types;

pub use types::{Command, RunOutput};

use std::time::Instant;

use crate::config::{discover_catalog_databases, ensure_catalog_databases_exist, Config};
use crate::domain::Workspace;
use crate::error::Result;
use crate::export::MigrationPlan;
use crate::timings::{self, PhaseTimings};

use database::run_command_for_database;
use merge::{merge_plan, merge_timings};

fn command_ensures_catalog_databases(cmd: types::Command) -> bool {
    matches!(
        cmd,
        types::Command::Migrate | types::Command::Baseline | types::Command::RepairChecksum
    )
}

pub async fn run_command(cmd: types::Command, cfg: &Config) -> Result<RunOutput> {
    if cmd == types::Command::Version {
        return Ok(types::RunOutput {
            exit_code: 0,
            timings: PhaseTimings::default(),
            plan: None,
        });
    }

    let cli_start = Instant::now();
    let databases = if !cfg.database.is_empty() {
        vec![cfg.database.clone()]
    } else {
        discover_catalog_databases(&cfg.sql_root)?
    };
    if command_ensures_catalog_databases(cmd) {
        ensure_catalog_databases_exist(cfg, &databases).await?;
    }

    let t_scan = Instant::now();
    let mut ws_full = Workspace::default();
    let scan_ms = crate::scan::populate(&mut ws_full, &cfg.sql_root, cfg.skip_git()).await?;
    let scan_elapsed = t_scan.elapsed();

    let mut merged = PhaseTimings {
        scan_ms,
        ..Default::default()
    };
    merged.set_l1_cache_hit(true);
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
        let out =
            run_command_for_database(cmd, &cfg_db, ws, scan_elapsed, cli_start, multi).await?;
        exit_code = exit_code.max(out.exit_code);
        merge_timings(&mut merged, &out.timings);
        if let Some(plan) = out.plan {
            merge_plan(&mut last_plan, plan);
        }
    }

    merged.plan_wall_ms = timings::dur_ms(cli_start.elapsed());
    merged.engine_ms = merged.plan_wall_ms.saturating_sub(merged.connect_ms);
    merged.cli_wall_ms = merged.plan_wall_ms;

    let out = types::RunOutput {
        exit_code,
        timings: merged,
        plan: last_plan,
    };
    Ok(out)
}
