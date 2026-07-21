use std::time::Duration;

use crate::audit;
use crate::domain::{Action, Workspace};
use crate::driver::TimingConn;
use crate::error::{Error, Result};
use crate::export::filter_applied_migrations_on_plan;
use crate::export::MigrationPlan;

pub async fn filter_applied(
    conn: &mut TimingConn,
    ws: &Workspace,
    plan: &mut MigrationPlan,
    timeout: Duration,
) -> Result<()> {
    let need = plan
        .objects
        .iter()
        .any(|o| o.planned_action == Action::ReprocessChanged && !o.transition_paths.is_empty());
    if !need {
        return Ok(());
    }
    let fut = audit::load_all_applied(conn.client_mut()?);
    let applied = if timeout.is_zero() {
        fut.await?
    } else {
        tokio::time::timeout(timeout, fut)
            .await
            .map_err(|_| Error::Sql(format!("history load timed out after {timeout:?}")))??
    };
    filter_applied_migrations_on_plan(plan, ws, &applied).map_err(|tampered| {
        Error::InvalidInput(format!(
            "applied transition script(s) modified after apply: {}; \
             restore the original contents or add a new transition",
            tampered.join(", ")
        ))
    })
}
