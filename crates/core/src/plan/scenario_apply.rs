use crate::domain::{Action, ObjectEntry};

use super::scenario::PlanScenario;

/// Resolves the planned action for an object given its `PlanScenario`, appending blocker messages as needed.
pub fn apply_scenario(
    scenario: PlanScenario,
    obj: &ObjectEntry,
    ws: &crate::domain::Workspace,
    child_row_id: u32,
    blockers: &mut Vec<String>,
) -> Action {
    let i = (child_row_id as usize).saturating_sub(1);
    match scenario {
        PlanScenario::TableBlockedNoTransitions => {
            blockers.push(format!(
                "table {} changed but has no non-scaffold transition scripts",
                obj.key_str(ws, i)
            ));
            scenario.action()
        }
        PlanScenario::TriggerBlockedParentMissing => {
            blockers.push(format!(
                "trigger {} parent table {} not found",
                obj.key_str(ws, i),
                parent_table_label(obj, ws, child_row_id)
            ));
            scenario.action()
        }
        PlanScenario::TriggerBlockedParentChanging => {
            blockers.push(format!(
                "trigger {} parent table {} is changing",
                obj.key_str(ws, i),
                parent_table_label(obj, ws, child_row_id)
            ));
            scenario.action()
        }
        _ => scenario.action(),
    }
}

fn parent_table_label(
    obj: &ObjectEntry,
    ws: &crate::domain::Workspace,
    child_row_id: u32,
) -> String {
    obj.parent_ref_for_row(ws, child_row_id)
        .filter(|p| p.parent_row_id > 0)
        .map(|p| {
            let pi = (p.parent_row_id as usize) - 1;
            ws.entry(pi).name_part(ws, pi).to_string()
        })
        .unwrap_or_else(|| "?".into())
}
