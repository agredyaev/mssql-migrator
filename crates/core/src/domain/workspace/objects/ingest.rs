use crate::domain::{store::finalize_sorted_entries, ObjectEntry, ObjectKey};

use super::super::Workspace;

fn entries_are_interned(entries: &[ObjectEntry]) -> bool {
    !entries.is_empty() && entries.iter().all(|e| e.key_off.1 != 0)
}

impl Workspace {
    pub fn push_object(&mut self, key: ObjectKey, obj: ObjectEntry) {
        if let Some(&id) = self.ingest_key_index.get(&key) {
            if id > 0 {
                let idx = id as usize - 1;
                self.object_entries[idx] = obj;
                self.cold.ingest_keys[idx] = key;
                return;
            }
        }
        let idx = self.object_entries.len();
        self.object_entries.push(obj);
        self.cold.ingest_keys.push(key.clone());
        self.ingest_key_index.insert(key, (idx + 1) as u32);
    }

    pub fn finalize_object_layout(&mut self) {
        if entries_are_interned(&self.object_entries) {
            self.rebuild_layout_from_interned();
            return;
        }
        let entries = std::mem::take(&mut self.object_entries);
        let keys = std::mem::take(&mut self.cold.ingest_keys);
        self.apply_finalized_entries(finalize_sorted_entries(entries, keys));
    }

    pub fn adopt_dense_entries(&mut self, pairs: Vec<(ObjectKey, ObjectEntry)>) {
        let (keys, entries): (Vec<_>, Vec<_>) = pairs.into_iter().unzip();
        if entries_are_interned(&entries) {
            self.object_entries = entries;
            self.cold.ingest_keys = keys;
            if self.layout_arena.is_some() {
                self.rebuild_layout_from_interned();
            }
            return;
        }
        self.apply_finalized_entries(finalize_sorted_entries(entries, keys));
    }
}
