use std::collections::HashMap;

use crate::domain::{ObjectKey, SharedStr, with_database_prefix};
use super::Workspace;

impl Workspace {
    pub(crate) fn object_path_at(&self, i: usize) -> SharedStr {
        if let Some(paths) = self.object_path_cache.as_ref() {
            if let Some(p) = paths.get(i) {
                return self.shared_at(*p);
            }
        }
        let obj = self.entry(i);
        with_database_prefix(self.database_name(obj.db_id).as_ref(), obj.script_path(self))
    }

    pub fn normalized_keys(&self) -> Vec<String> {
        self.object_entries
            .iter()
            .map(|o| o.key_str(self).to_string())
            .collect()
    }

    pub fn objects_by_path(&self) -> HashMap<String, ObjectKey> {
        let mut out = HashMap::with_capacity(self.object_entries.len());
        for obj in &self.object_entries {
            out.insert(obj.script_path(self).to_string(), obj.key(self));
        }
        out
    }

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
            .enumerate()
            .filter(|(_, o)| self.database_name(o.db_id).as_ref() == db)
            .map(|(i, o)| {
                let mut e = o.clone();
                e.staging_key = Some(self.entry_key(i).clone());
                e.key_off = crate::domain::StrOff::EMPTY;
                e
            })
            .collect();
        out.adopt_dense_entries(entries);
        let prefix = format!("{db}/");
        for id in 1..=self.script_rows.len() as u32 {
            let s = self.script(id);
            if s.path_str().starts_with(prefix.as_str()) {
                out.insert_script(self.script_to_ingest(id));
            }
        }
        for (&row_id, entries) in self.transitions_by_row.iter() {
            if row_id > 0 && row_id as usize <= out.object_entries.len() {
                out.transitions_by_row.insert(row_id, entries.clone());
            }
        }
        for (&row_id, parent) in self.parent_by_row.iter() {
            if row_id > 0 && row_id as usize <= out.object_entries.len() {
                out.parent_by_row.insert(row_id, parent.clone());
            }
        }
        crate::plan::rebuild_path_caches(&mut out);
        out
    }

    pub fn transition_paths_by_script(&self) -> HashMap<String, String> {
        let mut out = HashMap::new();
        for (&row_id, entries) in self.transitions_by_row.iter() {
            let table_key = self.entry_key(row_id as usize - 1).as_str();
            for e in entries {
                let s = self.script(e.script_id);
                out.insert(s.path_str().to_string(), table_key.to_string());
            }
        }
        out
    }
}
