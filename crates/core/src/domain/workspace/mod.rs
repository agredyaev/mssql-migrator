mod catalog;
mod compact;
mod objects;
mod reset;
mod scripts;
mod views;

use std::collections::HashMap;

use super::{ObjectEntry, ObjectKey, SchemaEntry, ScriptGit, ScriptKey, ScriptRow};

pub(crate) const CATALOG_APPLIED: u8 = 1 << 0;
pub(crate) const CHECKSUMS_APPLIED: u8 = 1 << 1;

/// In-memory repository layout and planning state.
#[derive(Clone, Debug)]
pub struct Workspace {
    /// Absolute repository root.
    pub root: String,
    /// Managed repository objects.
    pub object_entries: Vec<ObjectEntry>,
    /// Git metadata keyed by script id.
    pub script_git: HashMap<u32, ScriptGit>,
    /// Script id by repository-relative path.
    pub script_key_index: HashMap<ScriptKey, u32>,
    /// Object row id by normalized key.
    pub key_index: HashMap<ObjectKey, u32>,
    /// Duplicate detector used during scan.
    pub ingest_key_index: HashMap<(u16, ObjectKey), u32>,
    /// Staged transitions pending object-row assignment.
    pub transitions_staging: HashMap<(String, ObjectKey), Vec<(String, ScriptKey)>>,
    /// Schemas discovered during scan.
    pub schemas: Vec<SchemaEntry>,
    /// Script metadata.
    pub script_rows: Vec<ScriptRow>,
    /// Script checksums aligned with `script_rows`.
    pub script_checksums: Vec<Option<[u8; 32]>>,
    /// Database names indexed by `db_id`; slot zero is empty.
    pub database_names: Vec<String>,
    /// Finalized layout digest.
    pub layout_digest: [u8; 32],
    pub(crate) catalog_flags: u8,
}

impl Default for Workspace {
    fn default() -> Self {
        Self {
            root: String::new(),
            object_entries: Vec::new(),
            script_git: HashMap::new(),
            script_key_index: HashMap::new(),
            key_index: HashMap::new(),
            ingest_key_index: HashMap::new(),
            transitions_staging: HashMap::new(),
            schemas: Vec::new(),
            script_rows: Vec::new(),
            script_checksums: Vec::new(),
            database_names: vec![String::new()],
            layout_digest: [0; 32],
            catalog_flags: 0,
        }
    }
}

impl Workspace {
    /// Returns the 1-based row id for index `i`.
    pub fn row_id_at(&self, i: usize) -> u32 {
        (i + 1) as u32
    }

    /// Returns the number of managed objects.
    pub fn object_count(&self) -> usize {
        self.object_entries.len()
    }

    /// Returns the row id for `key`, or zero.
    pub fn key_index(&self, key: &ObjectKey) -> u32 {
        self.key_index.get(key).copied().unwrap_or(0)
    }
}
