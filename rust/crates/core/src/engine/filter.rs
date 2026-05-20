use crate::audit;
use crate::domain::Action;
use crate::driver::TimingConn;
use crate::error::Result;
use crate::export::MigrationPlan;
use crate::plan::filter_migrations;

pub async fn filter_applied(conn: &mut TimingConn, plan: &mut MigrationPlan) -> Result<()> {
    let need = plan.objects.iter().any(|o| {
        o.planned_action == Action::ReprocessChanged && !o.transition_paths.is_empty()
    });
    if !need {
        return Ok(());
    }
    let applied = audit::load_all_applied(&mut conn.inner).await?;
    filter_migrations::filter_applied_migrations(plan, &applied);
    Ok(())
}
