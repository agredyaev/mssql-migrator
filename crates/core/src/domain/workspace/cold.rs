use std::collections::HashMap;

use crate::domain::arena::LayoutArena;
use crate::domain::key::{ObjectKey, ScriptKey};
use crate::domain::object::ParentRef;
use crate::domain::schema::SchemaEntry;
use crate::domain::script::{ScriptGit, ScriptGitStaging, ScriptRow};
use crate::domain::shared::{empty_str, SharedStr};
use crate::domain::str_off::StrOff;
use crate::domain::transition::TransitionEntry;

/// Side store: maps, arena, scripts, caches.
#[derive(Clone, Debug, Default)]
pub struct WorkspaceCold {
    pub root: String,
    pub script_git: HashMap<u32, ScriptGit>,
    /// Git preload strings before arena intern (cleared in [`Self::clear_scan_staging`]).
    pub script_git_staging: HashMap<u32, ScriptGitStaging>,
    pub script_key_index: HashMap<ScriptKey, u32>,
    pub key_index: HashMap<ObjectKey, u32>,
    pub fp_index: HashMap<u64, u32>,
    /// Keyed by `(db_id, key)` so identical `<schema>/<kind>/<name>` objects in
    /// different catalog databases (multi-DB layout) do not collide; only a true
    /// in-database duplicate is rejected.
    pub ingest_key_index: HashMap<(u16, ObjectKey), u32>,
    /// Parallel to `object_entries` until layout finalize / intern.
    pub ingest_keys: Vec<ObjectKey>,
    /// Parallel to `script_rows` until script path intern.
    pub ingest_script_keys: Vec<ScriptKey>,
    pub ingest_script_abs: Vec<SharedStr>,
    pub transitions_by_row: HashMap<u32, Vec<TransitionEntry>>,
    pub transitions_staging: HashMap<ObjectKey, Vec<(SharedStr, ScriptKey)>>,
    pub parent_by_row: HashMap<u32, ParentRef>,
    pub parent_by_object: HashMap<ObjectKey, ParentRef>,
    pub transition_path_cache: Option<HashMap<u32, Vec<StrOff>>>,
    pub object_path_cache: Option<Vec<StrOff>>,
    pub layout_arena: Option<LayoutArena>,
    pub schemas: Vec<SchemaEntry>,
    pub script_rows: Vec<ScriptRow>,
    pub script_checksums: Vec<Option<[u8; 32]>>,
    pub database_names: Vec<SharedStr>,
    pub prior_by_row: Vec<Option<[u8; 32]>>,
    pub catalog_row: Vec<u8>,
    pub has_transition_paths_row: Vec<u8>,
    pub layout_digest: [u8; 32],
    pub string_arena_bytes: usize,
    pub string_arena_unique: usize,
}

impl WorkspaceCold {
    pub fn new() -> Self {
        let mut s = Self::default();
        s.database_names.push(empty_str());
        s
    }

    pub fn clear_object_staging(&mut self) {
        self.ingest_keys.clear();
        self.ingest_key_index.clear();
    }

    pub fn clear_scan_staging(&mut self) {
        self.clear_object_staging();
        if self
            .script_rows
            .iter()
            .all(|r| r.path_off.1 != 0 && r.abs_path_off.1 != 0)
        {
            self.ingest_script_keys.clear();
            self.ingest_script_abs.clear();
        }
    }
}
