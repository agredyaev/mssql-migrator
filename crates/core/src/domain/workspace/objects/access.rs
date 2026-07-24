use crate::domain::{ObjectEntry, ObjectKey};

use super::super::Workspace;

impl Workspace {
    /// Returns the object key at index `i`.
    pub fn entry_key(&self, i: usize) -> &ObjectKey {
        &self.object_entries[i].key
    }

    /// Returns the object at index `i`.
    pub fn entry(&self, i: usize) -> &ObjectEntry {
        &self.object_entries[i]
    }

    /// Returns the mutable object at index `i`.
    pub fn entry_mut(&mut self, i: usize) -> &mut ObjectEntry {
        &mut self.object_entries[i]
    }

    /// Returns the object for `key`.
    pub fn object_by_key(&self, key: &ObjectKey) -> Option<&ObjectEntry> {
        let id = self.key_index(key);
        self.object_entries.get(id.checked_sub(1)? as usize)
    }
}
