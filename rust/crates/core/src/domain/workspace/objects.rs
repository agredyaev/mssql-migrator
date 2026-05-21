use std::collections::HashMap;

use crate::domain::kind_code::kind_code;
use crate::domain::store::ObjectRow;
use crate::domain::{ObjectEntry, ObjectKey, store::finalize_sorted_entries};

use super::Workspace;

fn entries_are_interned(entries: &[ObjectEntry]) -> bool {
    !entries.is_empty() && entries.iter().all(|e| e.key_off.1 != 0)
}

impl Workspace {
    pub fn push_object(&mut self, obj: ObjectEntry) {
        let key = obj.key(self);
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

    pub fn finalize_object_layout(&mut self) {
        if entries_are_interned(&self.object_entries) {
            self.rebuild_layout_from_interned();
            return;
        }
        let entries = std::mem::take(&mut self.object_entries);
        self.apply_finalized_entries(finalize_sorted_entries(entries));
    }

    pub fn adopt_dense_entries(&mut self, entries: Vec<ObjectEntry>) {
        if entries_are_interned(&entries) {
            self.object_entries = entries;
            if self.layout_arena.is_some() {
                self.rebuild_layout_from_interned();
            }
            return;
        }
        self.apply_finalized_entries(finalize_sorted_entries(entries));
    }

    /// Build dense layout from aligned `object_keys` (e.g. [`super::Workspace::for_catalog_database`] clone).
    pub fn rebuild_layout_from_keys(&mut self, keys: Vec<ObjectKey>) {
        assert_eq!(keys.len(), self.object_entries.len());
        let n = keys.len();
        let mut schema_ids: HashMap<String, u16> = HashMap::new();
        let mut rows = Vec::with_capacity(n);
        let mut key_index = HashMap::with_capacity(n);
        let mut fp_index = HashMap::with_capacity(n);
        for (i, key) in keys.iter().enumerate() {
            let schema = key.schema_part();
            let schema_id = if schema.is_empty() {
                0
            } else {
                let next = (schema_ids.len() + 1) as u16;
                *schema_ids.entry(schema.to_string()).or_insert(next)
            };
            rows.push(ObjectRow {
                schema_id,
                kind_code: kind_code(key.kind_part()),
                flags: 0,
            });
            let row_id = (i + 1) as u32;
            fp_index.insert(key.fingerprint(), row_id);
            key_index.insert(key.clone(), row_id);
        }
        self.object_rows = rows;
        self.object_keys = keys;
        self.cold.key_index = key_index;
        self.cold.fp_index = fp_index;
        self.compact_sparse_maps();
        self.cold.clear_scan_staging();
        self.invalidate_catalog_facts();
        crate::plan::rebuild_path_caches(self);
    }

    /// Dense rows/keys from post-intern entries (`key_off` set, `staging_key` cleared).
    pub fn rebuild_layout_from_interned(&mut self) {
        let keys: Vec<_> = self
            .object_entries
            .iter()
            .map(|o| self.object_key(o.key_off))
            .collect();
        self.rebuild_layout_from_keys(keys);
    }

    fn apply_finalized_entries(
        &mut self,
        (rows, key_index, fp_index, entries, keys): (
            Vec<ObjectRow>,
            HashMap<ObjectKey, u32>,
            HashMap<u64, u32>,
            Vec<ObjectEntry>,
            Vec<ObjectKey>,
        ),
    ) {
        self.object_rows = rows;
        self.cold.key_index = key_index;
        self.cold.fp_index = fp_index;
        self.object_entries = entries;
        self.object_keys = keys;
        self.compact_sparse_maps();
        self.cold.clear_scan_staging();
        self.invalidate_catalog_facts();
        crate::plan::rebuild_path_caches(self);
    }

    pub fn entry_key(&self, i: usize) -> &ObjectKey {
        &self.object_keys[i]
    }

    pub fn entry(&self, i: usize) -> &ObjectEntry {
        &self.object_entries[i]
    }

    pub fn entry_mut(&mut self, i: usize) -> &mut ObjectEntry {
        &mut self.object_entries[i]
    }

    pub fn rebuild_fp_index(&mut self) {
        let n = self.object_keys.len();
        self.cold.fp_index.clear();
        self.cold.fp_index.reserve(n);
        for (i, key) in self.object_keys.iter().enumerate() {
            self.cold.fp_index.insert(key.fingerprint(), (i + 1) as u32);
        }
    }

    pub fn object_by_key(&self, key: &ObjectKey) -> Option<&ObjectEntry> {
        let id = self.key_index(key);
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
}
