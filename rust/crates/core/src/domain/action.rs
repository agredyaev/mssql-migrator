use serde::{Deserialize, Serialize};

#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[repr(u8)]
#[serde(rename_all = "snake_case")]
pub enum Action {
    CreateObject = 0,
    AdoptExisting = 1,
    SkipUnchanged = 2,
    UpdateExistingModule = 3,
    ReprocessChanged = 4,
    ReprocessChangedBlocked = 5,
    Fail = 6,
}

impl Action {
    pub const fn as_repr(self) -> u8 {
        self as u8
    }

    pub const fn from_repr(v: u8) -> Option<Self> {
        match v {
            0 => Some(Self::CreateObject),
            1 => Some(Self::AdoptExisting),
            2 => Some(Self::SkipUnchanged),
            3 => Some(Self::UpdateExistingModule),
            4 => Some(Self::ReprocessChanged),
            5 => Some(Self::ReprocessChangedBlocked),
            6 => Some(Self::Fail),
            _ => None,
        }
    }
}

#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum SchemaAction {
    CreateSchema,
    Exists,
}

pub fn is_transactional_kind(kind: &str) -> bool {
    matches!(
        kind,
        "tables" | "indexes" | "types" | "sequences" | "synonyms"
    )
}

pub fn is_module_kind(kind: &str) -> bool {
    matches!(kind, "views" | "procedures" | "functions" | "triggers")
}
