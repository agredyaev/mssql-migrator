use std::sync::{Arc, Mutex};
use std::time::Instant;

use crate::config::Config;
use crate::domain::Workspace;
use crate::driver::{connect, DbClient, TimingConn};
use crate::error::Result;
use crate::timings::{self, PhaseTimings};

use super::command_mutates;
use super::plan_phase::plan_phase;
use super::types::{Command, RunOutput};

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

    // Mutating commands inspect + diff + apply entirely under the advisory lock
    // (`apply_run::run_locked`) so a concurrent migrator cannot make the plan stale
    // between planning and apply. Read-only commands plan without the lock.
    let (plan, exit_code) = if command_mutates(cmd) {
        super::super::apply_run::run_locked(cmd, &mut conn, cfg, &mut ws, &mut timings).await?
    } else {
        let plan = plan_phase(cmd, cfg, &mut conn, &mut ws, &mut timings).await?;
        // Validate is the CI gate for "can this repository be applied": a
        // blocked plan is a failing validation, not a successful preview.
        let code = if cmd == Command::Validate && plan.blocked {
            crate::error::EXIT_PLAN_BLOCKED
        } else {
            0
        };
        (plan, code)
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
