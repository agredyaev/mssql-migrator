use crate::domain::ObjectEntry;
use crate::error::{Error, Result};

use super::super::Workspace;

impl Workspace {
    /// Inserts an object and rejects duplicate normalized keys within one database.
    pub fn push_object(&mut self, object: ObjectEntry) -> Result<()> {
        let index_key = (object.db_id, object.key.clone());
        if self.ingest_key_index.contains_key(&index_key) {
            return Err(Error::InvalidInput(format!(
                "duplicate object {:?} in database {:?}: two source files map to the same normalized <schema>/<kind>/<name> key (keys are case-insensitive); rename or remove one",
                object.key.as_str(),
                self.database_name(object.db_id),
            )));
        }
        let row_id = self.object_entries.len() as u32 + 1;
        self.ingest_key_index.insert(index_key, row_id);
        self.object_entries.push(object);
        Ok(())
    }

    /// Sorts objects by normalized key and rebuilds indexes.
    pub fn finalize_object_layout(&mut self) {
        self.object_entries
            .sort_by(|left, right| left.key.as_str().cmp(right.key.as_str()));
        self.key_index.clear();
        self.key_index.reserve(self.object_entries.len());
        for (index, object) in self.object_entries.iter().enumerate() {
            self.key_index.insert(object.key.clone(), index as u32 + 1);
        }
        self.compact_sparse_maps();
        self.ingest_key_index.clear();
        self.invalidate_catalog_facts();
    }

    /// Replaces the object list and rebuilds indexes.
    pub fn adopt_dense_entries(&mut self, objects: Vec<ObjectEntry>) {
        self.object_entries = objects;
        self.finalize_object_layout();
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::ObjectKey;

    #[test]
    fn push_object_rejects_duplicate_normalized_key() {
        let mut ws = Workspace::default();
        let object = ObjectEntry::new(
            ObjectKey::new("smoke", "tables", "Foo"),
            0,
            [0; 32],
            false,
            0,
        );
        ws.push_object(object).expect("first insert");
        let object = ObjectEntry::new(
            ObjectKey::new("smoke", "tables", "foo"),
            1,
            [0; 32],
            false,
            0,
        );
        assert!(ws.push_object(object).is_err());
    }
}
