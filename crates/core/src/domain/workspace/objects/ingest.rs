use crate::domain::{store::finalize_sorted_entries, ObjectEntry, ObjectKey};
use crate::error::{Error, Result};

use super::super::Workspace;

fn entries_are_interned(entries: &[ObjectEntry]) -> bool {
    !entries.is_empty() && entries.iter().all(|e| e.key_off.1 != 0)
}

impl Workspace {
    pub fn push_object(&mut self, key: ObjectKey, obj: ObjectEntry) -> Result<()> {
        let db_id = obj.db_id;
        let idx_key = (db_id, key.clone());
        if self.ingest_key_index.contains_key(&idx_key) {
            // Two source files in the same catalog database normalize to the same
            // `<schema>/<kind>/<name>` key (the key is lowercased, so case-only
            // variants collide too). Identical keys in *different* databases are
            // legitimate in a multi-DB layout and are kept distinct by `db_id`.
            // Silently overwriting would drop one object from the migration plan,
            // so fail closed with a clear, deterministic error instead.
            return Err(Error::InvalidInput(format!(
                "duplicate object {:?} in database {:?}: two source files map to the same \
                 normalized <schema>/<kind>/<name> key (keys are case-insensitive); \
                 rename or remove one",
                key.as_str(),
                self.database_name(db_id).as_ref(),
            )));
        }
        let idx = self.object_entries.len();
        self.object_entries.push(obj);
        self.cold.ingest_keys.push(key.clone());
        self.ingest_key_index.insert(idx_key, (idx + 1) as u32);
        Ok(())
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn push_object_rejects_duplicate_normalized_key() {
        let mut ws = Workspace::default();
        let (k1, o1) = ObjectEntry::with_staging_key(
            ObjectKey::new("smoke", "tables", "Foo"),
            0,
            [0u8; 32],
            false,
            0,
        );
        ws.push_object(k1, o1).expect("first insert succeeds");

        // `Foo` and `foo` normalize to the same key. The duplicate must be a hard
        // error, not a silent overwrite that drops one object from the plan.
        let (k2, o2) = ObjectEntry::with_staging_key(
            ObjectKey::new("smoke", "tables", "foo"),
            1,
            [0u8; 32],
            false,
            0,
        );
        let err = ws
            .push_object(k2, o2)
            .expect_err("duplicate normalized key must error");
        assert!(
            err.to_string().contains("duplicate object"),
            "unexpected error: {err}"
        );
    }
}
