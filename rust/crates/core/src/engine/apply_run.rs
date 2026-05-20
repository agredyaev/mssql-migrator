use std::time::Instant;

use crate::config::Config;
use crate::domain::Workspace;
use crate::driver::TimingConn;
use crate::error::{Error, Result};
use crate::export::MigrationPlan;
use crate::timings::{self, PhaseTimings};

use super::filter;
use super::run::Command;

pub async fn maybe_apply(
    cmd: Command,
    conn: &mut TimingConn,
    cfg: &Config,
    ws: &Workspace,
    plan: &mut MigrationPlan,
    timings: &mut PhaseTimings,
) -> Result<()> {
    match cmd {
        Command::Plan | Command::Validate | Command::Version => Ok(()),
        Command::Migrate => {
            if plan.blocked {
                return super::blocked::handle_blocked_migrate(conn, cfg, ws, plan).await;
            }
            filter::filter_applied(conn, plan).await?;
            run_apply(conn, cfg, ws, plan, timings).await
        }
        Command::Baseline | Command::RepairChecksum => {
            if plan.blocked {
                return Err(Error::PlanBlocked);
            }
            run_apply(conn, cfg, ws, plan, timings).await
        }
    }
}

async fn run_apply(
    conn: &mut TimingConn,
    cfg: &Config,
    ws: &Workspace,
    plan: &MigrationPlan,
    timings: &mut PhaseTimings,
) -> Result<()> {
    crate::lock::acquire(conn, cfg).await?;
    let t = Instant::now();
    let apply = crate::apply::execute_plan(cfg, conn, ws, plan).await?;
    timings.apply_ms = timings::dur_ms(t.elapsed());
    crate::lock::release(conn).await?;
    eprintln!(
        "apply: applied={} skipped={} failed={}",
        apply.applied, apply.skipped, apply.failed
    );
    if apply.failed == 0 && apply.applied > 0 {
        let _ = crate::db::save_workspace_snapshot(conn, &ws.layout_digest, ws).await;
    }
    Ok(())
}
