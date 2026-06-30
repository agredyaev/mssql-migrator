use crate::domain::{ObjectEntry, ObjectKey};

use super::super::Workspace;

impl Workspace {
    /// Returns the object key at index `i`.
    pub fn entry_key(&self, i: usize) -> &ObjectKey {
        if i < self.object_keys.len() {
            &self.object_keys[i]
        } else {
            &self.cold.ingest_keys[i]
        }
    }

    /// Returns a reference to the object entry at index `i`.
    pub fn entry(&self, i: usize) -> &ObjectEntry {
        &self.object_entries[i]
    }

    /// Returns a mutable reference to the object entry at index `i`.
    pub fn entry_mut(&mut self, i: usize) -> &mut ObjectEntry {
        &mut self.object_entries[i]
    }

    /// Rebuilds the fingerprint-to-index map from the current object keys.
    pub fn rebuild_fp_index(&mut self) {
        let n = self.object_keys.len();
        self.cold.fp_index.clear();
        self.cold.fp_index.reserve(n);
        for (i, key) in self.object_keys.iter().enumerate() {
            self.cold.fp_index.insert(key.fingerprint(), (i + 1) as u32);
        }
    }

    /// Returns the entry for `key`, or `None` if the key is not present.
    pub fn object_by_key(&self, key: &ObjectKey) -> Option<&ObjectEntry> {
        let id = self.key_index(key);
        if id > 0 {
            return self.object_entries.get(id as usize - 1);
        }
        None
    }

    /// Calls `f` for each object entry in the workspace.
    pub fn for_each_entry<F>(&self, mut f: F)
    where
        F: FnMut(&ObjectEntry),
    {
        for obj in &self.object_entries {
            f(obj);
        }
    }

    /// Calls `f` mutably for each object entry in the workspace.
    pub fn for_each_entry_mut<F>(&mut self, mut f: F)
    where
        F: FnMut(&mut ObjectEntry),
    {
        for obj in &mut self.object_entries {
            f(obj);
        }
    }
}
