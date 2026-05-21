use crate::domain::{Action, ObjectEntry};

use super::scenario::PlanScenario;

pub fn apply_scenario(
    scenario: PlanScenario,
    obj: &ObjectEntry,
    ws: &crate::domain::Workspace,
    child_row_id: u32,
    blockers: &mut Vec<String>,
) -> Action {
    match scenario {
        PlanScenario::TableBlockedNoTransitions => {
            blockers.push(format!(
                "table {} changed but has no non-scaffold transition scripts",
                obj.key_str(ws)
            ));
            scenario.action()
        }
        PlanScenario::TriggerBlockedParentMissing => {
            blockers.push(format!(
                "trigger {} parent table {} not found",
                obj.key_str(ws),
                parent_table_label(obj, ws, child_row_id)
            ));
            scenario.action()
        }
        PlanScenario::TriggerBlockedParentChanging => {
            blockers.push(format!(
                "trigger {} parent table {} is changing",
                obj.key_str(ws),
                parent_table_label(obj, ws, child_row_id)
            ));
            scenario.action()
        }
        _ => scenario.action(),
    }
}

fn parent_table_label(obj: &ObjectEntry, ws: &crate::domain::Workspace, child_row_id: u32) -> String {
    obj.parent_ref_for_row(ws, child_row_id)
        .filter(|p| p.parent_row_id > 0)
        .map(|p| ws.entry((p.parent_row_id as usize) - 1).name_part(ws).to_string())
        .unwrap_or_else(|| "?".into())
}
