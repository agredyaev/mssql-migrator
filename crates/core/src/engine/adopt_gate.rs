use crate::config::Config;
use crate::domain::{Action, Workspace};
use crate::error::{Error, Result};
use crate::export::MigrationPlan;

/// Adoption trusts a live object by name alone (no structure or definition
/// check), so implicit adoption during `migrate` requires explicit opt-in via
/// `RMIG_ALLOW_ADOPT`; `rmig baseline` remains the always-explicit path.
pub(super) fn ensure_adopt_allowed(
    cfg: &Config,
    ws: &Workspace,
    plan: &mut MigrationPlan,
) -> Result<()> {
    if cfg.allow_adopt() {
        return Ok(());
    }
    plan.ensure_objects_materialized(ws);
    let adopts: Vec<&str> = plan
        .objects
        .iter()
        .filter(|o| o.planned_action == Action::AdoptExisting)
        .map(|o| o.normalized_key.as_ref())
        .collect();
    if adopts.is_empty() {
        return Ok(());
    }
    tracing::error!(
        count = adopts.len(),
        objects = ?adopts.iter().take(10).collect::<Vec<_>>(),
        "migrate would adopt existing objects without verifying their definitions; \
         set RMIG_ALLOW_ADOPT=1 to allow, or run 'rmig baseline' to adopt explicitly"
    );
    Err(Error::PlanBlocked)
}
