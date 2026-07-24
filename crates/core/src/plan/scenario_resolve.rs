use crate::domain::{is_module_kind_code, ObjectEntry, KIND_TABLES, KIND_TRIGGERS};

use super::scenario::PlanScenario;

/// Input bundle passed to `resolve_plan_scenario`.
pub struct ScenarioInput<'a> {
    /// Whether the object currently exists in the database.
    pub exists: bool,
    /// Prior checksum recorded in the catalog, if any.
    pub prior: Option<[u8; 32]>,
    /// Current on-disk checksum of the script.
    pub checksum: [u8; 32],
    /// Numeric kind code for the object type.
    pub kind_code: u8,
    /// Object entry describing the migration target.
    pub obj: &'a ObjectEntry,
    /// Workspace containing the full object registry.
    pub ws: &'a crate::domain::Workspace,
    /// Whether the object has at least one transition-path row.
    pub has_transition_paths: bool,
    /// The audited live-state fingerprint differs from SQL Server's current state.
    pub live_definition_drift: bool,
}

/// Resolves the `PlanScenario` to apply for a single object given `input`.
pub fn resolve_plan_scenario(input: ScenarioInput<'_>) -> PlanScenario {
    let ScenarioInput {
        exists,
        prior,
        checksum,
        kind_code,
        obj,
        ws,
        has_transition_paths,
        live_definition_drift,
    } = input;
    if !exists {
        return PlanScenario::Create;
    }
    if prior.is_none() || prior == Some([0; 32]) {
        return PlanScenario::Adopt;
    }
    if live_definition_drift {
        return if is_module_kind_code(kind_code) {
            PlanScenario::ModuleUpdate
        } else {
            PlanScenario::LiveStructuralDriftBlocked
        };
    }
    if prior == Some(checksum) {
        return PlanScenario::SkipUnchanged;
    }
    resolve_changed_scenario(kind_code, obj, ws, has_transition_paths)
}

fn resolve_changed_scenario(
    kind_code: u8,
    obj: &ObjectEntry,
    ws: &crate::domain::Workspace,
    has_transition_paths: bool,
) -> PlanScenario {
    match kind_code {
        KIND_TABLES => {
            if !has_transition_paths {
                PlanScenario::TableBlockedNoTransitions
            } else {
                PlanScenario::TableReprocess
            }
        }
        KIND_TRIGGERS => {
            let Some(pref) = obj.parent else {
                return changed_default_scenario(kind_code);
            };
            let parent_row_id = pref.parent_row_id;
            if parent_row_id == 0 {
                return PlanScenario::TriggerBlockedParentMissing;
            }
            let parent_i = parent_row_id as usize - 1;
            if !ws.catalog_has_row(parent_i) {
                return PlanScenario::TriggerBlockedParentMissing;
            }
            if ws
                .entry(parent_i)
                .prior_checksum
                .is_none_or(|checksum| checksum == [0; 32])
            {
                return PlanScenario::TriggerBlockedParentChanging;
            }
            PlanScenario::TriggerUpdateModule
        }
        _ => changed_default_scenario(kind_code),
    }
}

fn changed_default_scenario(kind_code: u8) -> PlanScenario {
    if is_module_kind_code(kind_code) {
        PlanScenario::ModuleUpdate
    } else {
        PlanScenario::StructuralChangeBlocked
    }
}
