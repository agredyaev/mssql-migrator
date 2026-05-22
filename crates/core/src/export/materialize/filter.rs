use std::collections::HashMap;

use crate::domain::{Action, Workspace};

use crate::export::plan_json::MigrationPlan;

pub fn filter_applied_migrations_on_plan(
    plan: &mut MigrationPlan,
    ws: &Workspace,
    applied: &HashMap<String, bool>,
) {
    if plan.rows.is_empty() {
        return filter_legacy_objects(plan, applied);
    }
    let need = plan.rows.iter().enumerate().any(|(i, row)| {
        row.planned_action() == Action::ReprocessChanged
            && plan
                .plan_transitions
                .get(&(i as u32))
                .is_some_and(|v| !v.is_empty())
    });
    if !need {
        return;
    }
    for (i, row) in plan.rows.iter().enumerate() {
        if row.planned_action() != Action::ReprocessChanged {
            continue;
        }
        let Some(paths) = plan.plan_transitions.get_mut(&(i as u32)) else {
            continue;
        };
        paths.retain(|off| {
            let p: &str = ws.str_at(*off);
            !applied.contains_key(p)
        });
    }
    plan.objects.clear();
}

fn filter_legacy_objects(plan: &mut MigrationPlan, applied: &HashMap<String, bool>) {
    let need = plan
        .objects
        .iter()
        .any(|o| o.planned_action == Action::ReprocessChanged && !o.transition_paths.is_empty());
    if !need {
        return;
    }
    for obj in &mut plan.objects {
        if obj.planned_action != Action::ReprocessChanged || obj.transition_paths.is_empty() {
            continue;
        }
        obj.transition_paths
            .retain(|tp| !applied.contains_key(tp.as_ref()));
    }
}
