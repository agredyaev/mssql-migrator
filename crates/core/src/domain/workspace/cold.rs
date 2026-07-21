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
    /// Absolute path to the repository root directory.
    pub root: String,
    /// Git metadata keyed by script row index.
    pub script_git: HashMap<u32, ScriptGit>,
    /// Git preload strings before arena intern (cleared in [`Self::clear_scan_staging`]).
    pub script_git_staging: HashMap<u32, ScriptGitStaging>,
    /// Maps script keys to their row index.
    pub script_key_index: HashMap<ScriptKey, u32>,
    /// Maps object keys to their row index.
    pub key_index: HashMap<ObjectKey, u32>,
    /// Maps file path hashes to object row indices.
    pub fp_index: HashMap<u64, u32>,
    /// Keyed by `(db_id, key)` so identical `<schema>/<kind>/<name>` objects in
    /// different catalog databases (multi-DB layout) do not collide; only a true
    /// in-database duplicate is rejected.
    pub ingest_key_index: HashMap<(u16, ObjectKey), u32>,
    /// Parallel to `object_entries` until layout finalize / intern.
    pub ingest_keys: Vec<ObjectKey>,
    /// Parallel to `script_rows` until script path intern.
    pub ingest_script_keys: Vec<ScriptKey>,
    /// Absolute paths for scripts pending intern.
    pub ingest_script_abs: Vec<SharedStr>,
    /// Transition entries grouped by object row index.
    pub transitions_by_row: HashMap<u32, Vec<TransitionEntry>>,
    /// Staged transition data pending row assignment.
    pub transitions_staging: HashMap<(SharedStr, ObjectKey), Vec<(SharedStr, ScriptKey)>>,
    /// Parent references keyed by child object row index.
    pub parent_by_row: HashMap<u32, ParentRef>,
    /// Cached string offsets for transition paths, populated after layout finalize.
    pub transition_path_cache: Option<HashMap<u32, Vec<StrOff>>>,
    /// Cached string offsets for object paths, populated after layout finalize.
    pub object_path_cache: Option<Vec<StrOff>>,
    /// Interned string arena used for layout path segments.
    pub layout_arena: Option<LayoutArena>,
    /// Schema entries discovered during scan.
    pub schemas: Vec<SchemaEntry>,
    /// Dense list of script metadata rows.
    pub script_rows: Vec<ScriptRow>,
    /// SHA-256 checksums per script row; `None` if not yet computed.
    pub script_checksums: Vec<Option<[u8; 32]>>,
    /// Database names indexed by catalog `db_id`; slot 0 is always empty.
    pub database_names: Vec<SharedStr>,
    /// Previously recorded checksums per object row for change detection.
    pub prior_by_row: Vec<Option<[u8; 32]>>,
    /// Flags whether each object row originates from a catalog database.
    pub catalog_row: Vec<u8>,
    /// Flags whether each object row has associated transition paths.
    pub has_transition_paths_row: Vec<u8>,
    /// SHA-256 digest of the finalized layout, used for cache invalidation.
    pub layout_digest: [u8; 32],
    /// Total bytes stored in the shared string arena.
    pub string_arena_bytes: usize,
    /// Number of unique strings stored in the shared string arena.
    pub string_arena_unique: usize,
}

impl WorkspaceCold {
    /// Creates a default workspace cold store with slot 0 of `database_names` pre-populated.
    pub fn new() -> Self {
        let mut s = Self::default();
        s.database_names.push(empty_str());
        s
    }

    /// Clears object ingest staging buffers.
    pub fn clear_object_staging(&mut self) {
        self.ingest_keys.clear();
        self.ingest_key_index.clear();
    }

    /// Clears object and script staging buffers after all paths are interned.
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
