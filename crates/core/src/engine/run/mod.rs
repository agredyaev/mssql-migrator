mod database;
mod merge;
pub(super) mod plan_phase;
mod planned_at;
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

/// Commands that mutate the database: they run inside the advisory lock and
/// require their catalog databases to exist first.
pub(super) fn command_mutates(cmd: types::Command) -> bool {
    matches!(
        cmd,
        types::Command::Migrate | types::Command::Baseline | types::Command::RepairChecksum
    )
}

fn command_ensures_catalog_databases(cmd: types::Command) -> bool {
    command_mutates(cmd)
}

/// Runs `cmd` for all configured catalog databases and returns the merged output.
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
        // In a multi-DB layout the catalog list is discovered from the folder
        // structure, so a directory may name a database that does not exist on
        // the server yet. Read-only commands cannot inspect a missing database
        // and must not create it, so plan the reachable catalogs only and skip
        // the rest. Mutating commands already created their catalogs above via
        // `ensure_catalog_databases_exist`, so this probe is skipped for them.
        if multi && !command_ensures_catalog_databases(cmd) {
            // Absent database → skip; a server outage/auth failure must NOT be
            // silently treated as "absent" (that would exit 0 having validated
            // nothing), so propagate it.
            match crate::config::target_database_exists(cfg, db).await? {
                true => {}
                false => {
                    tracing::warn!(database = %db, "skipping catalog database that does not exist on server");
                    continue;
                }
            }
        }
        match run_command_for_database(cmd, &cfg_db, ws, scan_elapsed, cli_start, multi).await {
            Ok(out) => {
                exit_code = exit_code.max(out.exit_code);
                merge_timings(&mut merged, &out.timings);
                if let Some(plan) = out.plan {
                    merge_plan(&mut last_plan, plan);
                }
            }
            // In a multi-DB run the databases are independent; don't let one
            // failure discard the databases already processed. Record its exit
            // code, log which database failed, and continue. Single-DB runs keep
            // propagating the error unchanged.
            Err(e) if multi => {
                tracing::error!(database = %db, error = %e, "catalog database failed; continuing");
                exit_code = exit_code.max(e.exit_code());
            }
            Err(e) => return Err(e),
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

#[cfg(test)]
#[path = "../../tests/command_mutates_test.rs"]
mod command_mutates_tests;
