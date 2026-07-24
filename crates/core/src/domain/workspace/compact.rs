use std::collections::HashMap;

use crate::domain::{ObjectKey, ScriptKey, TransitionEntry};
use crate::error::{Error, Result};

use super::Workspace;

impl Workspace {
    pub(crate) fn compact_sparse_maps(&mut self) {
        let staging = std::mem::take(&mut self.transitions_staging);
        for object in &mut self.object_entries {
            object.transitions.clear();
        }
        let rows: HashMap<(u16, ObjectKey), usize> = self
            .object_entries
            .iter()
            .enumerate()
            .map(|(index, object)| ((object.db_id, object.key.clone()), index))
            .collect();
        for ((database, key), entries) in staging {
            let db_id = self.intern_database(database);
            let Some(&index) = rows.get(&(db_id, key.clone())) else {
                tracing::warn!(
                    table = key.as_str(),
                    "dropping staged transitions: no matching table object"
                );
                continue;
            };
            let mut transitions = Vec::with_capacity(entries.len());
            for (ordinal, script_key) in entries {
                let Some(script_id) = self.script_key_index.get(&script_key).copied() else {
                    tracing::warn!(
                        table = key.as_str(),
                        "dropping transition: script key not registered"
                    );
                    continue;
                };
                transitions.push(TransitionEntry::new(ordinal, script_id));
            }
            transitions.sort_by(|left, right| left.ordinal.cmp(&right.ordinal));
            self.object_entries[index].transitions = transitions;
        }
    }

    /// Stages a transition script for a table.
    pub fn push_transition_staging(
        &mut self,
        database: String,
        table_key: ObjectKey,
        ordinal: String,
        script_key: ScriptKey,
    ) -> Result<()> {
        let staged = (database, table_key);
        if self
            .transitions_staging
            .get(&staged)
            .is_some_and(|entries| entries.iter().any(|(value, _)| value == &ordinal))
        {
            return Err(Error::InvalidInput(format!(
                "duplicate transition ordinal {ordinal} for {}/{}: each migration ordinal must be unique per table",
                staged.0,
                staged.1.as_str()
            )));
        }
        self.transitions_staging
            .entry(staged)
            .or_default()
            .push((ordinal, script_key));
        Ok(())
    }
}
