use std::time::Instant;

use crate::config::Config;
use crate::domain::Workspace;
use crate::driver::TimingConn;
use crate::error::{Error, Result};
use crate::export::MigrationPlan;
use crate::timings::{self, PhaseTimings};

use super::adopt_gate::ensure_adopt_allowed;
use super::filter;
use super::run::plan_phase::plan_phase;
use super::run::Command;

/// Acquire the advisory lock, then plan (inspect + diff) and apply entirely under
/// it, so a concurrent migrator cannot make the plan stale between planning and
/// apply. The lock is always released after the body (BG-001) via
/// `release_after_body`. Returns the applied plan and the process exit code.
pub async fn run_locked(
    cmd: Command,
    conn: &mut TimingConn,
    cfg: &Config,
    ws: &mut Workspace,
    timings: &mut PhaseTimings,
) -> Result<(MigrationPlan, i32)> {
    crate::lock::acquire(conn, cfg).await?;
    let body_result = locked_body(cmd, conn, cfg, ws, timings).await;
    crate::lock::release_after_body(conn, body_result).await
}

async fn locked_body(
    cmd: Command,
    conn: &mut TimingConn,
    cfg: &Config,
    ws: &mut Workspace,
    timings: &mut PhaseTimings,
) -> Result<(MigrationPlan, i32)> {
    let mut plan = plan_phase(cmd, cfg, conn, ws, timings).await?;
    let exit_code = match apply_plan(cmd, conn, cfg, ws, &mut plan, timings).await {
        Err(Error::PlanBlocked) if cmd == Command::Migrate => crate::error::EXIT_PLAN_BLOCKED,
        Err(e) => return Err(e),
        Ok(()) => 0,
    };
    Ok((plan, exit_code))
}

async fn apply_plan(
    cmd: Command,
    conn: &mut TimingConn,
    cfg: &Config,
    ws: &Workspace,
    plan: &mut MigrationPlan,
    timings: &mut PhaseTimings,
) -> Result<()> {
    match cmd {
        Command::Migrate => {
            if plan.blocked {
                return super::blocked::handle_blocked_migrate(conn, cfg, ws, plan).await;
            }
            ensure_adopt_allowed(cfg, ws, plan)?;
            filter::filter_applied(conn, ws, plan, cfg.command_timeout).await?;
        }
        Command::Baseline | Command::RepairChecksum => {
            // Both are audit-metadata commands: never execute repository DDL,
            // module bodies, or transitions through the generic executor.
            let mode = if cmd == Command::Baseline {
                crate::apply::MetadataMode::Baseline
            } else {
                crate::apply::MetadataMode::RepairChecksum
            };
            let t = Instant::now();
            let apply = crate::apply::execute_metadata_plan(cfg, conn, ws, plan, mode).await?;
            timings.apply_ms = timings::dur_ms(t.elapsed());
            tracing::debug!(skipped = apply.skipped, "metadata apply finished");
            return Ok(());
        }
        Command::Plan | Command::Validate | Command::Version => return Ok(()),
    }
    let t = Instant::now();
    let apply = crate::apply::execute_plan(cfg, conn, ws, plan).await?;
    timings.apply_ms = timings::dur_ms(t.elapsed());
    tracing::debug!(
        applied = apply.applied,
        skipped = apply.skipped,
        failed = apply.failed,
        "apply finished"
    );
    if apply.failed == 0 && apply.applied > 0 {
        super::warm_store::clear_plan_db_snapshot();
        let _ =
            crate::db::save_workspace_snapshot(conn, &ws.layout_digest, ws, cfg.catalog_cache())
                .await;
    }
    Ok(())
}
