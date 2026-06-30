use crate::domain::key::{ObjectKey, ScriptKey};
use crate::domain::transition::TransitionEntry;
use crate::domain::SharedStr;
use crate::error::{Error, Result};

use super::Workspace;

impl Workspace {
    pub(crate) fn compact_sparse_maps(&mut self) {
        self.compact_transitions();
        self.compact_parents();
    }

    fn compact_transitions(&mut self) {
        let staging = std::mem::take(&mut self.transitions_staging);
        self.transitions_by_row.clear();
        for (key, entries) in staging {
            let row_id = self.key_index(&key);
            if row_id == 0 {
                continue;
            }
            let mut row_entries = Vec::with_capacity(entries.len());
            for (ord, sk) in entries {
                let Some(script_id) = self.script_key_index.get(&sk).copied() else {
                    continue;
                };
                row_entries.push(TransitionEntry::new_staging(ord, script_id));
            }
            if !row_entries.is_empty() {
                self.transitions_by_row.insert(row_id, row_entries);
            }
        }
    }

    fn compact_parents(&mut self) {
        let staging = std::mem::take(&mut self.parent_by_object);
        self.parent_by_row.clear();
        for (key, parent) in staging {
            let row_id = self.key_index(&key);
            if row_id > 0 {
                self.parent_by_row.insert(row_id, parent);
            }
        }
    }

    /// Appends a transition script to the staging map for `table_key`.
    pub fn push_transition_staging(
        &mut self,
        table_key: ObjectKey,
        ordinal: SharedStr,
        script_key: ScriptKey,
    ) -> Result<()> {
        // Reject two migration files claiming the same ordinal for one table: the
        // apply order would be ambiguous. Gaps are intentionally allowed (apply
        // tolerates them); only duplicates are a contract violation.
        if let Some(entries) = self.transitions_staging.get(&table_key) {
            if entries.iter().any(|(ord, _)| *ord == ordinal) {
                return Err(Error::InvalidInput(format!(
                    "duplicate transition ordinal {} for {}: each migration ordinal must be unique per table",
                    &*ordinal,
                    table_key.as_str()
                )));
            }
        }
        self.transitions_staging
            .entry(table_key)
            .or_default()
            .push((ordinal, script_key));
        Ok(())
    }
}
