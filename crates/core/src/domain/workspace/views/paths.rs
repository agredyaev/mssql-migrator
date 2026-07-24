use std::collections::HashMap;

use super::super::Workspace;
use crate::domain::{with_database_prefix, ObjectKey};

impl Workspace {
    pub(crate) fn object_path_at(&self, index: usize) -> String {
        let object = self.entry(index);
        with_database_prefix(
            self.database_name(object.db_id),
            self.script(object.script_id).path_str(),
        )
    }

    /// Maps script paths to object keys.
    pub fn objects_by_path(&self) -> HashMap<String, ObjectKey> {
        self.object_entries
            .iter()
            .map(|object| {
                (
                    self.script(object.script_id).path_str().to_owned(),
                    object.key.clone(),
                )
            })
            .collect()
    }

    /// Maps transition script paths to owner table keys.
    pub fn transition_paths_by_script(&self) -> HashMap<String, String> {
        let mut out = HashMap::new();
        for object in &self.object_entries {
            for transition in &object.transitions {
                out.insert(
                    self.script(transition.script_id).path_str().to_owned(),
                    object.key.as_str().to_owned(),
                );
            }
        }
        out
    }
}
