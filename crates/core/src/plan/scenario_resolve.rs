use crate::db::state::CatalogState;
use crate::domain::{is_module_kind_code, ObjectEntry, KIND_TABLES, KIND_TRIGGERS};

use super::diff_prepare::prior_digest_present;
use super::scenario::PlanScenario;

pub struct ScenarioInput<'a> {
    pub exists: bool,
    pub prior: Option<[u8; 32]>,
    pub checksum: [u8; 32],
    pub kind_code: u8,
    pub obj: &'a ObjectEntry,
    pub ws: &'a crate::domain::Workspace,
    pub catalog: &'a CatalogState,
    pub prior_digests: &'a [Option<[u8; 32]>],
    pub child_row_id: u32,
    pub has_transition_paths: bool,
}

pub fn resolve_plan_scenario(input: ScenarioInput<'_>) -> PlanScenario {
    let ScenarioInput {
        exists,
        prior,
        checksum,
        kind_code,
        obj,
        ws,
        catalog,
        prior_digests,
        child_row_id,
        has_transition_paths,
    } = input;
    if !exists {
        return PlanScenario::Create;
    }
    if prior.is_none() || prior == Some([0; 32]) {
        return PlanScenario::Adopt;
    }
    if prior == Some(checksum) {
        return PlanScenario::SkipUnchanged;
    }
    resolve_changed_scenario(
        kind_code,
        obj,
        ws,
        catalog,
        prior_digests,
        child_row_id,
        has_transition_paths,
    )
}

fn resolve_changed_scenario(
    kind_code: u8,
    obj: &ObjectEntry,
    ws: &crate::domain::Workspace,
    _catalog: &CatalogState,
    prior_digests: &[Option<[u8; 32]>],
    child_row_id: u32,
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
        KIND_TRIGGERS if obj.parent_ref_for_row(ws, child_row_id).is_some() => {
            let Some(pref) = obj.parent_ref_for_row(ws, child_row_id) else {
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
            if !prior_digest_present(prior_digests, parent_i) {
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
        PlanScenario::Reprocess
    }
}
