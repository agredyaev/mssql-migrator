use std::collections::HashMap;

use crate::domain::Workspace;
use crate::export::MigrationPlan;

pub fn filter_applied_migrations(
    plan: &mut MigrationPlan,
    ws: &Workspace,
    applied: &HashMap<String, bool>,
) {
    crate::export::filter_applied_migrations_on_plan(plan, ws, applied);
}
