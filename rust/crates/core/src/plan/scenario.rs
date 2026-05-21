use crate::domain::Action;

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
