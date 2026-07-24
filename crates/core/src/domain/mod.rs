//! Repository objects, scripts, schemas, and workspace state.
//!
//! ### Purpose
//! Declares the primary structures and data models representing database objects, schemas, transition
//! scripts, and workspaces that support diff planning.
//!
//! ### Architectural Context
//! - **Inputs**: SQL files parsed from disk, catalog state entries.
//! - **Outputs**: Strongly-typed structures (`Workspace`, `ObjectEntry`, `SchemaEntry`).
//! - **Boundaries**: SQL bodies stay on disk; metadata uses ordinary owned strings.
//!
//! ### Nominal Flow
//! 1. Ingest workspace files into ordinary object and script vectors.
//! 2. Build the domain layout representing catalog tables, views, procedures, and transitions.
//! 3. Use standard `ObjectKey` and `ScriptKey` indexes to perform O(1) comparison audits.
//!
//! ### Off-Nominal & Failure Containment
//! - **Invalid Layout**: Scan returns `Error::InvalidInput` before planning.

mod action;
mod databases;
mod key;
mod kind_code;
mod layout_path;
mod object;
mod schema;
mod script;
mod script_resolve;
mod transition;
mod workspace;

pub use action::{is_transactional_kind, Action, SchemaAction};
pub use key::{ObjectKey, ScriptKey};
pub use kind_code::{
    is_module_kind_code, kind_code, KIND_FUNCTIONS, KIND_INDEXES, KIND_PROCEDURES, KIND_SEQUENCES,
    KIND_SYNONYMS, KIND_TABLES, KIND_TRIGGERS, KIND_TYPES, KIND_VIEWS,
};
pub use layout_path::{object_path_for_entry, path_lookup_candidates, with_database_prefix};
pub use object::{ObjectEntry, ParentRef};
pub use schema::SchemaEntry;
pub use script::{Script, ScriptGit, ScriptKind, ScriptRow};
pub use script_resolve::ScriptRef;
pub use transition::TransitionEntry;
pub use workspace::Workspace;
