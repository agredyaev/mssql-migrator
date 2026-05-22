use crate::audit;
use crate::domain::{Action, Workspace};
use crate::driver::TimingConn;
use crate::error::Result;
use crate::export::MigrationPlan;
use crate::plan::filter_migrations;

pub async fn filter_applied(
    conn: &mut TimingConn,
    ws: &Workspace,
    plan: &mut MigrationPlan,
) -> Result<()> {
    let need = plan
        .objects
        .iter()
        .any(|o| o.planned_action == Action::ReprocessChanged && !o.transition_paths.is_empty());
    if !need {
        return Ok(());
    }
    let applied = audit::load_all_applied(conn.client_mut()).await?;
    filter_migrations::filter_applied_migrations(plan, ws, &applied);
    Ok(())
}
