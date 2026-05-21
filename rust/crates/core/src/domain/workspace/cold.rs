use std::collections::HashMap;

use crate::domain::arena::LayoutArena;
use crate::domain::key::{ObjectKey, ScriptKey};
use crate::domain::object::ParentRef;
use crate::domain::schema::SchemaEntry;
use crate::domain::script::{ScriptGit, ScriptRow};
use crate::domain::shared::{empty_str, SharedStr};
use crate::domain::str_off::StrOff;
use crate::domain::transition::TransitionEntry;

/// Cold layout side store (**COLD** / **SLAB**): maps, arena, scripts, caches.
#[derive(Clone, Debug, Default)]
pub struct WorkspaceCold {
    pub root: String,
    pub layout_digest: [u8; 32],
    pub schemas: Vec<SchemaEntry>,
    pub script_rows: Vec<ScriptRow>,
    pub script_checksums: Vec<Option<[u8; 32]>>,
    pub script_git: HashMap<u32, ScriptGit>,
    pub script_key_index: HashMap<ScriptKey, u32>,
    pub key_index: HashMap<ObjectKey, u32>,
    /// Normalized-key fingerprint → 1-based dense row id (**IDX** / plan DB).
    pub fp_index: HashMap<u64, u32>,
    pub ingest_key_index: HashMap<ObjectKey, u32>,
    pub transitions_by_row: HashMap<u32, Vec<TransitionEntry>>,
    pub transitions_staging: HashMap<ObjectKey, Vec<(SharedStr, ScriptKey)>>,
    pub parent_by_row: HashMap<u32, ParentRef>,
    pub parent_by_object: HashMap<ObjectKey, ParentRef>,
    pub transition_path_cache: Option<HashMap<u32, Vec<StrOff>>>,
    pub object_path_cache: Option<Vec<StrOff>>,
    pub layout_arena: Option<LayoutArena>,
    pub string_arena_bytes: usize,
    pub string_arena_unique: usize,
    pub database_names: Vec<SharedStr>,
    /// Audit prior digests by dense row index (built in [`crate::plan::scope::apply_checksums_if_needed`]).
    pub prior_by_row: Vec<Option<[u8; 32]>>,
    /// `1` when `CatalogState.objects` contains this row's key (set in `apply_catalog`).
    pub catalog_row: Vec<u8>,
    /// `1` when row has non-empty transition path cache entry (tables).
    pub has_transition_paths_row: Vec<u8>,
}

impl WorkspaceCold {
    pub fn new() -> Self {
        Self {
            database_names: vec![empty_str()],
            ..Default::default()
        }
    }

    /// Scan-only maps; drop after [`super::Workspace::finalize_object_layout`] (**TAG**).
    pub fn clear_scan_staging(&mut self) {
        self.transitions_staging.clear();
        self.parent_by_object.clear();
        self.ingest_key_index.clear();
    }
}
