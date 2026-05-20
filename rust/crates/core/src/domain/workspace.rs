use std::collections::HashMap;

use super::key::{ObjectKey, ScriptKey};
use super::object::ObjectEntry;
use super::schema::SchemaEntry;
use super::script::Script;
use super::shared::SharedStr;
use super::store::{finalize_sorted_entries, ObjectStore};

#[derive(Clone, Debug, Default)]
pub struct Workspace {
    pub root: String,
    pub layout_digest: [u8; 32],
    pub schemas: Vec<SchemaEntry>,
    pub scripts: HashMap<ScriptKey, Script>,
    pub object_entries: Vec<ObjectEntry>,
    pub object_store: ObjectStore,
    /// Upsert index during scan ingest only; cleared in [`Self::finalize_object_layout`].
    ingest_key_index: HashMap<ObjectKey, u32>,
    pub transitions_by_table: HashMap<ObjectKey, Vec<(SharedStr, ScriptKey)>>,
    pub(crate) transition_path_cache: Option<HashMap<ObjectKey, Vec<SharedStr>>>,
    pub blocked: bool,
    /// [`Self::mark_catalog_applied`] / [`Self::mark_checksums_applied`] — skip re-scan on warmed diff.
    catalog_flags: u8,
    pub string_arena_bytes: usize,
    pub string_arena_unique: usize,
}

const CATALOG_APPLIED: u8 = 1 << 0;
const CHECKSUMS_APPLIED: u8 = 1 << 1;

impl Workspace {
    /// Append or replace a layout object during scan ingest (before finalize).
    pub fn push_object(&mut self, obj: ObjectEntry) {
        let key = obj.key.clone();
        if let Some(&id) = self.ingest_key_index.get(&key) {
            if id > 0 {
                self.object_entries[id as usize - 1] = obj;
                return;
            }
        }
        let idx = self.object_entries.len();
        self.object_entries.push(obj);
        self.ingest_key_index.insert(key, (idx + 1) as u32);
    }

    /// Sort entries and build dense [`ObjectStore`] + key index (scan finalize / diff guard).
    pub fn finalize_object_layout(&mut self) {
        let entries = std::mem::take(&mut self.object_entries);
        let (store, entries) = finalize_sorted_entries(entries);
        self.object_store = store;
        self.object_entries = entries;
        self.ingest_key_index.clear();
        self.invalidate_catalog_facts();
    }

    pub fn adopt_dense_entries(&mut self, entries: Vec<ObjectEntry>) {
        let (store, entries) = finalize_sorted_entries(entries);
        self.object_store = store;
        self.object_entries = entries;
        self.ingest_key_index.clear();
        self.invalidate_catalog_facts();
    }

    pub fn invalidate_transition_paths(&mut self) {
        self.transition_path_cache = None;
    }

    pub fn invalidate_catalog_facts(&mut self) {
        self.catalog_flags &= !(CATALOG_APPLIED | CHECKSUMS_APPLIED);
    }

    pub fn catalog_applied(&self) -> bool {
        self.catalog_flags & CATALOG_APPLIED != 0
    }

    pub fn mark_catalog_applied(&mut self) {
        self.catalog_flags |= CATALOG_APPLIED;
    }

    pub fn checksums_applied(&self) -> bool {
        self.catalog_flags & CHECKSUMS_APPLIED != 0
    }

    pub fn mark_checksums_applied(&mut self) {
        self.catalog_flags |= CHECKSUMS_APPLIED;
    }

    /// Clear layout object state before a full rescan.
    pub fn reset_layout(&mut self) {
        self.object_entries.clear();
        self.ingest_key_index.clear();
        self.object_store = ObjectStore::default();
        self.scripts.clear();
        self.transitions_by_table.clear();
        self.invalidate_transition_paths();
        self.schemas.clear();
        self.string_arena_bytes = 0;
        self.string_arena_unique = 0;
        self.invalidate_catalog_facts();
    }

    pub fn object_count(&self) -> usize {
        self.object_entries.len()
    }

    pub fn entry(&self, i: usize) -> &ObjectEntry {
        &self.object_entries[i]
    }

    pub fn entry_mut(&mut self, i: usize) -> &mut ObjectEntry {
        &mut self.object_entries[i]
    }

    pub fn object_by_key(&self, key: &ObjectKey) -> Option<&ObjectEntry> {
        let id = self.object_store.key_index(key);
        if id > 0 {
            return self.object_entries.get(id as usize - 1);
        }
        None
    }

    pub fn for_each_entry<F>(&self, mut f: F)
    where
        F: FnMut(&ObjectEntry),
    {
        for obj in &self.object_entries {
            f(obj);
        }
    }

    pub fn for_each_entry_mut<F>(&mut self, mut f: F)
    where
        F: FnMut(&mut ObjectEntry),
    {
        for obj in &mut self.object_entries {
            f(obj);
        }
    }

    pub fn normalized_keys(&self) -> Vec<String> {
        self.object_entries
            .iter()
            .map(|o| o.key.as_str().to_string())
            .collect()
    }

    pub fn objects_by_path(&self) -> HashMap<String, ObjectKey> {
        let mut out = HashMap::with_capacity(self.object_entries.len());
        for obj in &self.object_entries {
            out.insert(obj.script.as_str().to_string(), obj.key.clone());
        }
        out
    }

    /// View of layout state for one catalog database (`sql_root/<db>/...`).
    pub fn for_catalog_database(&self, db: &str) -> Workspace {
        let mut out = Workspace::default();
        out.root = self.root.clone();
        out.layout_digest = self.layout_digest;
        out.schemas = self
            .schemas
            .iter()
            .filter(|s| s.database.as_ref() == db)
            .cloned()
            .collect();
        let entries: Vec<_> = self
            .object_entries
            .iter()
            .filter(|o| o.database_name.as_ref() == db)
            .cloned()
            .collect();
        out.adopt_dense_entries(entries);
        let prefix = format!("{db}/");
        for (k, script) in &self.scripts {
            if k.as_str().starts_with(prefix.as_str()) {
                out.scripts.insert(k.clone(), script.clone());
            }
        }
        for (table_key, entries) in &self.transitions_by_table {
            if out.object_store.key_index(table_key) > 0 {
                out.transitions_by_table
                    .insert(table_key.clone(), entries.clone());
            }
        }
        crate::plan::rebuild_transition_path_cache(&mut out);
        out
    }

    pub fn transition_paths_by_script(&self) -> HashMap<String, String> {
        let mut out = HashMap::new();
        for (table_key, entries) in &self.transitions_by_table {
            for (_, sk) in entries {
                if let Some(s) = self.scripts.get(sk) {
                    out.insert(
                        s.key.as_str().to_string(),
                        table_key.as_str().to_string(),
                    );
                }
            }
        }
        out
    }
}
