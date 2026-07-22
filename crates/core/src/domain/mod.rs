//! Entities, arenas, object stores, and workspace representation.
//!
//! ### Purpose
//! Declares the primary structures and data models representing database objects, schemas, transition
//! scripts, arena allocators, string interners, and workspaces that support diff planning.
//!
//! ### Architectural Context
//! - **Inputs**: SQL files parsed from disk, catalog state entries.
//! - **Outputs**: Strongly-typed structures (`Workspace`, `ObjectEntry`, `SchemaEntry`).
//! - **Boundaries**: Uses string arenas (`LayoutArena`, `StringArena`) to optimize heap usage.
//!
//! ### Nominal Flow
//! 1. Ingest workspace files, allocating strings into string interners.
//! 2. Build the domain layout representing catalog tables, views, procedures, and transitions.
//! 3. Use standard `ObjectKey` and `ScriptKey` indexes to perform O(1) comparison audits.
//!
//! ### Off-Nominal & Failure Containment
//! - **String Interning Limit**: Arena capacities are bounded. Reaching limits triggers `Error::InvalidInput`.

mod action;
mod arena;
mod arena_resolve;
mod databases;
mod fingerprint;
mod key;
mod kind_code;
mod layout_path;
mod object;
mod path_cache;
mod schema;
mod script;
mod script_resolve;
mod shared;
mod store;
mod str_off;
mod transition;
mod workspace;

pub use action::{is_transactional_kind, Action, SchemaAction};
pub use arena::{
    install_layout_arena, intern_script_git_strings, intern_workspace_strings, LayoutArena,
    StringArena, StringArenaBuilder,
};
pub use fingerprint::key_fingerprint;
pub use key::{ObjectKey, ScriptKey};
pub use kind_code::{
    is_module_kind_code, kind_code, KIND_FUNCTIONS, KIND_INDEXES, KIND_PROCEDURES, KIND_SEQUENCES,
    KIND_SYNONYMS, KIND_TABLES, KIND_TRIGGERS, KIND_TYPES, KIND_VIEWS,
};
pub use layout_path::{object_path_for_entry, path_lookup_candidates, with_database_prefix};
pub use object::{ObjectEntry, ParentRef};
pub use path_cache::{ensure_path_caches, rebuild_path_caches};
pub use schema::SchemaEntry;
pub use script::{
    Script, ScriptGit, ScriptKind, ScriptRow, SCRIPT_FLAG_HAS_CHECKSUM, SCRIPT_FLAG_SCAFFOLD,
};
pub use script_resolve::ScriptRef;
pub use shared::{empty_str, share, SharedStr};
pub use store::ObjectRow;
pub use str_off::StrOff;
pub use transition::TransitionEntry;
pub use workspace::{Workspace, WorkspaceCold};
