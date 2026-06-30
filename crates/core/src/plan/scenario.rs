use crate::domain::Action;

/// Planned action classification for each database object.
///
/// `#[repr(u8)]` — compact wire encoding; enables `as_repr`/`from_repr`
/// conversions without a serde round-trip.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
#[repr(u8)]
pub enum PlanScenario {
    /// Object does not exist; will be created from script.
    Create = 0,
    /// Object exists with no recorded checksum; adopted as baseline.
    Adopt = 1,
    /// Object checksum matches current script; no action needed.
    SkipUnchanged = 2,
    /// Table has changed and will be reprocessed via transitions.
    TableReprocess = 3,
    /// Table has changed but no transition scripts are defined; blocked.
    TableBlockedNoTransitions = 4,
    /// Trigger body has changed; module will be updated in place.
    TriggerUpdateModule = 5,
    /// Trigger's parent table is absent; blocked until parent is present.
    TriggerBlockedParentMissing = 6,
    /// Trigger's parent table is concurrently changing; blocked.
    TriggerBlockedParentChanging = 7,
    /// Non-table module has changed and will be updated in place.
    ModuleUpdate = 8,
    /// Object has changed and will be reprocessed.
    Reprocess = 9,
}

impl PlanScenario {
    /// Returns the `Action` that corresponds to this scenario.
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

    /// Returns `true` if this scenario requires Git metadata to be written.
    pub fn with_git(self) -> bool {
        !matches!(self, Self::SkipUnchanged)
    }

    /// Returns 1 if this scenario is a blocked state, 0 otherwise.
    pub fn blocked_delta(self) -> u32 {
        match self {
            Self::TableBlockedNoTransitions
            | Self::TriggerBlockedParentMissing
            | Self::TriggerBlockedParentChanging => 1,
            _ => 0,
        }
    }

    /// Returns the `CounterKind` bucket for plan summary counters.
    pub fn counter_kind(self) -> CounterKind {
        match self {
            Self::Create => CounterKind::Create,
            Self::Adopt => CounterKind::Adopt,
            Self::SkipUnchanged => CounterKind::Skip,
            _ => CounterKind::Changed,
        }
    }
}

/// Summary counter bucket used when tallying plan results.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum CounterKind {
    /// Object will be created.
    Create,
    /// Object will be adopted from the database.
    Adopt,
    /// Object is unchanged and will be skipped.
    Skip,
    /// Object has changed and will be reprocessed or updated.
    Changed,
}
