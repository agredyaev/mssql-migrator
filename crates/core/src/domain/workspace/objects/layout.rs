use std::collections::HashMap;

use crate::domain::kind_code::kind_code;
use crate::domain::store::ObjectRow;
use crate::domain::ObjectKey;

use super::super::Workspace;

impl Workspace {
    /// Build dense layout from aligned `object_keys` (e.g. [`Workspace::for_catalog_database`] clone).
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
        crate::domain::rebuild_path_caches(self);
    }

    /// Dense rows/keys from post-intern entries (`key_off` set, ingest keys cleared).
    pub fn rebuild_layout_from_interned(&mut self) {
        let keys: Vec<_> = self
            .object_entries
            .iter()
            .map(|o| self.object_key(o.key_off))
            .collect();
        self.rebuild_layout_from_keys(keys);
    }

    pub(super) fn apply_finalized_entries(
        &mut self,
        (rows, key_index, fp_index, entries, keys): crate::domain::store::FinalizeSortedResult,
    ) {
        self.object_rows = rows;
        self.cold.key_index = key_index;
        self.cold.fp_index = fp_index;
        self.object_entries = entries;
        self.object_keys = keys;
        self.compact_sparse_maps();
        self.cold.clear_scan_staging();
        self.invalidate_catalog_facts();
        crate::domain::rebuild_path_caches(self);
    }
}
