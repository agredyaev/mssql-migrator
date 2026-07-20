//! [`filter_applied_migrations`] — removes already-applied transitions from the plan.

use std::collections::HashMap;

use crate::domain::Workspace;
use crate::export::MigrationPlan;

/// Removes already-applied transition paths from `plan`. Returns the list of
/// applied paths whose current file contents no longer match the recorded
/// checksum (tampered history).
pub fn filter_applied_migrations(
    plan: &mut MigrationPlan,
    ws: &Workspace,
    applied: &HashMap<String, String>,
) -> Result<(), Vec<String>> {
    crate::export::filter_applied_migrations_on_plan(plan, ws, applied)
}
