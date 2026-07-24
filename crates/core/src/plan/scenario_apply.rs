use crate::domain::{Action, ObjectEntry};

use super::scenario::PlanScenario;

/// Resolves the planned action for an object given its `PlanScenario`, appending blocker messages as needed.
pub fn apply_scenario(
    scenario: PlanScenario,
    obj: &ObjectEntry,
    ws: &crate::domain::Workspace,
    blockers: &mut Vec<String>,
) -> Action {
    match scenario {
        PlanScenario::TableBlockedNoTransitions => {
            blockers.push(format!(
                "table {} changed but has no non-scaffold transition scripts",
                obj.key
            ));
            scenario.action()
        }
        PlanScenario::StructuralChangeBlocked => {
            blockers.push(format!(
                "{} changed but this object kind has no safe in-place update path",
                obj.key
            ));
            scenario.action()
        }
        PlanScenario::LiveStructuralDriftBlocked => {
            blockers.push(format!(
                "live {} differs from its last audited structural state",
                obj.key
            ));
            scenario.action()
        }
        PlanScenario::TriggerBlockedParentMissing => {
            blockers.push(format!(
                "trigger {} parent table {} not found",
                obj.key,
                parent_table_label(obj, ws)
            ));
            scenario.action()
        }
        PlanScenario::TriggerBlockedParentChanging => {
            blockers.push(format!(
                "trigger {} parent table {} is changing",
                obj.key,
                parent_table_label(obj, ws)
            ));
            scenario.action()
        }
        _ => scenario.action(),
    }
}

fn parent_table_label(obj: &ObjectEntry, ws: &crate::domain::Workspace) -> String {
    obj.parent
        .filter(|p| p.parent_row_id > 0)
        .map(|p| {
            let pi = (p.parent_row_id as usize) - 1;
            ws.entry(pi).key.name_shared()
        })
        .unwrap_or_else(|| "?".into())
}
