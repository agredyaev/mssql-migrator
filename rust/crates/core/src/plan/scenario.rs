use crate::db::state::{CatalogState, ChecksumMap};
use crate::domain::{
    is_module_kind_code, Action, ObjectEntry, ObjectKey, KIND_TABLES, KIND_TRIGGERS,
};

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
#[repr(u8)]
pub enum PlanScenario {
    Create = 0,
    Adopt = 1,
    SkipUnchanged = 2,
    TableReprocess = 3,
    TableBlockedNoTransitions = 4,
    TriggerUpdateModule = 5,
    TriggerBlockedParentMissing = 6,
    TriggerBlockedParentChanging = 7,
    ModuleUpdate = 8,
    Reprocess = 9,
}

impl PlanScenario {
    pub fn action(self) -> Action {
        match self {
            Self::Create => Action::CreateObject,
            Self::Adopt => Action::AdoptExisting,
            Self::SkipUnchanged => Action::SkipUnchanged,
            Self::TableReprocess | Self::Reprocess => Action::ReprocessChanged,
            Self::TableBlockedNoTransitions
            | Self::TriggerBlockedParentMissing
            | Self::TriggerBlockedParentChanging => Action::ReprocessChangedBlocked,
            Self::TriggerUpdateModule | Self::ModuleUpdate => Action::UpdateExistingModule,
        }
    }

    pub fn with_git(self) -> bool {
        !matches!(self, Self::SkipUnchanged)
    }

    pub fn blocked_delta(self) -> u32 {
        match self {
            Self::TableBlockedNoTransitions
            | Self::TriggerBlockedParentMissing
            | Self::TriggerBlockedParentChanging => 1,
            _ => 0,
        }
    }

    pub fn counter_kind(self) -> CounterKind {
        match self {
            Self::Create => CounterKind::Create,
            Self::Adopt => CounterKind::Adopt,
            Self::SkipUnchanged => CounterKind::Skip,
            _ => CounterKind::Changed,
        }
    }
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum CounterKind {
    Create,
    Adopt,
    Skip,
    Changed,
}

pub fn resolve_plan_scenario(
    exists: bool,
    prior: Option<[u8; 32]>,
    checksum: [u8; 32],
    kind_code: u8,
    obj: &ObjectEntry,
    catalog: &CatalogState,
    checksums: &ChecksumMap,
    has_transition_paths: bool,
) -> PlanScenario {
    if !exists {
        return PlanScenario::Create;
    }
    if prior.is_none() || prior == Some([0; 32]) {
        return PlanScenario::Adopt;
    }
    if prior == Some(checksum) {
        return PlanScenario::SkipUnchanged;
    }
    resolve_changed_scenario(kind_code, obj, catalog, checksums, has_transition_paths)
}

fn resolve_changed_scenario(
    kind_code: u8,
    obj: &ObjectEntry,
    catalog: &CatalogState,
    checksums: &ChecksumMap,
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
        KIND_TRIGGERS if !obj.parent_name.is_empty() => {
            let Some(parent_key) = obj.parent_key.as_ref() else {
                return changed_default_scenario(kind_code);
            };
            if !catalog.objects.contains_key(parent_key) {
                return PlanScenario::TriggerBlockedParentMissing;
            }
            if !prior_digest_present(checksums, parent_key) {
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

pub fn apply_scenario(
    scenario: PlanScenario,
    obj: &ObjectEntry,
    blockers: &mut Vec<String>,
) -> Action {
    match scenario {
        PlanScenario::TableBlockedNoTransitions => {
            blockers.push(format!(
                "table {} changed but has no non-scaffold transition scripts",
                obj.key.as_str()
            ));
            scenario.action()
        }
        PlanScenario::TriggerBlockedParentMissing => {
            let parent_key = obj
                .parent_key
                .as_ref()
                .map(|k| k.as_str())
                .unwrap_or("");
            blockers.push(format!(
                "trigger {} parent table {} not found",
                obj.key.as_str(),
                parent_key
            ));
            scenario.action()
        }
        PlanScenario::TriggerBlockedParentChanging => {
            let parent_key = obj
                .parent_key
                .as_ref()
                .map(|k| k.as_str())
                .unwrap_or("");
            blockers.push(format!(
                "trigger {} parent table {} is changing",
                obj.key.as_str(),
                parent_key
            ));
            scenario.action()
        }
        _ => scenario.action(),
    }
}

fn prior_digest_present(m: &ChecksumMap, key: &ObjectKey) -> bool {
    m.get(key).is_some_and(|cs| *cs != [0; 32])
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::{empty_str, ObjectKey, ScriptKey};

    #[test]
    fn skip_when_checksum_matches() {
        let key = ObjectKey::new("s", "views", "v");
        let obj = ObjectEntry {
            key: key.clone(),
            script: ScriptKey::from_path("db/s/views/v.sql"),
            history: None,
            db: Default::default(),
            plan: None,
            checksum: [1; 32],
            schema: "s".into(),
            kind: "views".into(),
            name: "v".into(),
            database_name: "db".into(),
            parent_name: empty_str(),
            parent_key: None,
        };
        let s = resolve_plan_scenario(
            true,
            Some([1; 32]),
            [1; 32],
            6,
            &obj,
            &CatalogState::default(),
            &ChecksumMap::new(),
            false,
        );
        assert_eq!(s, PlanScenario::SkipUnchanged);
    }
}
