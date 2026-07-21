use std::io::Write;

use crate::domain::Workspace;
use crate::error::Result;

use super::types::MigrationPlan;

/// Serializes `plan` as pretty-printed JSON and writes it to `w`.
pub fn write_plan_json(
    plan: &MigrationPlan,
    ws: Option<&Workspace>,
    w: &mut dyn Write,
) -> Result<()> {
    let fully_materialized = !plan.rows.is_empty() && plan.objects.len() == plan.rows.len();
    let v = if !plan.rows.is_empty() && !fully_materialized {
        let ws = ws.ok_or_else(|| {
            crate::error::Error::InvalidInput(
                "write_plan_json: workspace required for slim plan rows".into(),
            )
        })?;
        serde_json::to_string_pretty(&crate::export::materialize::WireMigrationPlan::new(
            plan, ws,
        ))
    } else {
        serde_json::to_string_pretty(&crate::export::materialize::PlanJsonFromObjects(plan))
    }
    .map_err(|e| crate::error::Error::Other(e.into()))?;
    w.write_all(v.as_bytes()).map_err(crate::error::Error::Io)?;
    w.write_all(b"\n").map_err(crate::error::Error::Io)?;
    Ok(())
}
