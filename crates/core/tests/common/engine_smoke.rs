//! Engine helpers for cold apply / warm plan smoke tests (no git).

use migrator_core::config::Config;
use migrator_core::engine::{run_command, Command, RunOutput};
use migrator_core::error::Result;
use migrator_core::export::MigrationPlan;
use migrator_core::timings::PhaseTimings;

pub async fn baseline_migrate(cfg: &Config) -> Result<RunOutput> {
    let mut c = cfg.clone();
    c.skip_git = true;
    c.session_socket.clear();
    // Tests using this helper opt into implicit adoption (the operator's
    // RMIG_ALLOW_ADOPT); the adoption gate itself is covered by
    // apply_integrity_integration.
    c.allow_adopt = true;
    run_command(Command::Migrate, &c).await
}

pub async fn plan(cfg: &Config) -> Result<(MigrationPlan, PhaseTimings)> {
    let mut c = cfg.clone();
    c.skip_git = true;
    c.session_socket.clear();
    let out = run_command(Command::Plan, &c).await?;
    let plan = out
        .plan
        .ok_or_else(|| migrator_core::error::Error::Other(anyhow::anyhow!("missing plan")))?;
    Ok((plan, out.timings))
}
