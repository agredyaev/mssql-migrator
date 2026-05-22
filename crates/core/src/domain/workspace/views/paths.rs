use std::collections::HashMap;

use super::super::Workspace;
use crate::domain::{with_database_prefix, ObjectKey, SharedStr};

impl Workspace {
    pub(crate) fn object_path_at(&self, i: usize) -> SharedStr {
        if let Some(paths) = self.object_path_cache.as_ref() {
            if let Some(p) = paths.get(i) {
                return self.shared_at(*p);
            }
        }
        let obj = self.entry(i);
        with_database_prefix(
            self.database_name(obj.db_id).as_ref(),
            obj.script_path(self),
        )
    }

    pub fn objects_by_path(&self) -> HashMap<String, ObjectKey> {
        let mut out = HashMap::with_capacity(self.object_entries.len());
        for (i, obj) in self.object_entries.iter().enumerate() {
            out.insert(obj.script_path(self).to_string(), self.entry_key(i).clone());
        }
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
