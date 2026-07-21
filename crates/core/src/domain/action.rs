//! [`Action`] and [`SchemaAction`] — planned operation for each database object.
//!
//! ### Purpose
//! `Action` encodes what `rmig` intends to do with a database object
//! (create, adopt, skip, update, reprocess, fail). `SchemaAction` tracks
//! whether a schema needs creation. Helper functions classify kinds by their
//! transactional or module behaviour.
//!
//! ### Serialisation
//! Both enums serialise as snake-case strings via `serde(rename_all = "snake_case")`.

use serde::{Deserialize, Serialize};

/// Planned action for a single database object.
///
/// `#[repr(u8)]` — zero-cost numeric conversion for compact serialization
/// (`as_repr` / `from_repr`).
#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[repr(u8)]
#[serde(rename_all = "snake_case")]
pub enum Action {
    /// Object does not exist in DB → create it.
    CreateObject = 0,
    /// Object exists but no checksum → adopt as baseline.
    AdoptExisting = 1,
    /// Object unchanged → skip.
    SkipUnchanged = 2,
    /// Module (view/proc/func/trigger) needs update.
    UpdateExistingModule = 3,
    /// Object changed and can be reprocessed.
    ReprocessChanged = 4,
    /// Object changed but dependencies block reprocessing.
    ReprocessChangedBlocked = 5,
    /// Object cannot be processed (fatal).
    Fail = 6,
}

impl Action {
    /// Numeric representation (`u8`).
    pub const fn as_repr(self) -> u8 {
        self as u8
    }

    /// Decode from numeric representation; returns `None` for unknown values.
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

/// Planned action for a schema (create or already exists).
///
/// `#[repr(u8)]` — zero-cost numeric conversion for compact serialization.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize, Deserialize)]
#[repr(u8)]
#[serde(rename_all = "snake_case")]
pub enum SchemaAction {
    /// Schema needs to be created in the database.
    CreateSchema = 0,
    /// Schema already exists.
    Exists = 1,
}

/// True when the kind supports transactional DDL (`tables`, `indexes`, `types`, `sequences`, `synonyms`).
pub fn is_transactional_kind(kind: &str) -> bool {
    matches!(
        kind,
        "tables" | "indexes" | "types" | "sequences" | "synonyms"
    )
}
