use std::collections::HashMap;

use crate::domain::Action;
use crate::export::MigrationPlan;

pub fn filter_applied_migrations(plan: &mut MigrationPlan, applied: &HashMap<String, bool>) {
    let need = plan.objects.iter().any(|o| {
        o.planned_action == Action::ReprocessChanged && !o.transition_paths.is_empty()
    });
    if !need {
        return;
    }
    for obj in &mut plan.objects {
        if obj.planned_action != Action::ReprocessChanged || obj.transition_paths.is_empty() {
            continue;
        }
        obj.transition_paths.retain(|tp| !applied.contains_key(tp.as_ref()));
    }
}
