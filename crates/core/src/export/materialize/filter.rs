use std::collections::HashMap;

use crate::domain::{Action, ScriptKind, Workspace};

use crate::export::plan_json::MigrationPlan;

/// Removes already-applied transition paths from `plan`, updating either slim
/// rows or legacy objects. An applied path whose current file checksum no
/// longer matches the recorded one is TAMPERED history — collected into the
/// error list instead of being silently dropped.
pub fn filter_applied_migrations_on_plan(
    plan: &mut MigrationPlan,
    ws: &Workspace,
    applied: &HashMap<String, String>,
) -> Result<(), Vec<String>> {
    let current = transition_checksums(ws);
    let mut tampered: Vec<String> = Vec::new();
    if plan.rows.is_empty() {
        filter_legacy_objects(plan, applied, &current, &mut tampered);
    } else {
        filter_rows(plan, ws, applied, &current, &mut tampered);
    }
    if tampered.is_empty() {
        Ok(())
    } else {
        tampered.sort();
        Err(tampered)
    }
}

/// True when `path` was applied before AND its bytes are unchanged (an empty
/// recorded checksum is legacy data — trusted as applied, nothing to compare).
fn applied_unchanged(
    path: &str,
    applied: &HashMap<String, String>,
    current: &HashMap<String, String>,
    tampered: &mut Vec<String>,
) -> bool {
    let Some(recorded) = applied.get(path) else {
        return false;
    };
    match current.get(path) {
        Some(now) if !recorded.is_empty() && now != recorded => {
            tampered.push(path.to_string());
            true
        }
        _ => true,
    }
}

fn transition_checksums(ws: &Workspace) -> HashMap<String, String> {
    ws.scripts_iter()
        .filter(|s| s.kind() == ScriptKind::Transition)
        .filter_map(|s| {
            s.checksum()
                .map(|cs| (s.path_str().to_string(), hex::encode(cs)))
        })
        .collect()
}

fn filter_rows(
    plan: &mut MigrationPlan,
    ws: &Workspace,
    applied: &HashMap<String, String>,
    current: &HashMap<String, String>,
    tampered: &mut Vec<String>,
) {
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
            !applied_unchanged(p, applied, current, tampered)
        });
    }
    plan.objects.clear();
}

fn filter_legacy_objects(
    plan: &mut MigrationPlan,
    applied: &HashMap<String, String>,
    current: &HashMap<String, String>,
    tampered: &mut Vec<String>,
) {
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
            .retain(|tp| !applied_unchanged(tp.as_ref(), applied, current, tampered));
    }
}
