use serde::{Deserialize, Serialize};

#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum Action {
    CreateObject,
    AdoptExisting,
    SkipUnchanged,
    UpdateExistingModule,
    ReprocessChanged,
    ReprocessChangedBlocked,
    Fail,
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
