use crate::domain::key::{ObjectKey, ScriptKey};
use crate::domain::transition::TransitionEntry;
use crate::domain::SharedStr;
use crate::error::{Error, Result};

use super::Workspace;

impl Workspace {
    pub(crate) fn compact_sparse_maps(&mut self) {
        self.compact_transitions();
    }

    fn compact_transitions(&mut self) {
        let staging = std::mem::take(&mut self.transitions_staging);
        self.transitions_by_row.clear();
        // Same-named tables can exist in several catalog databases; resolve
        // each staged transition to the row of ITS database, not to whichever
        // duplicate key happens to win in the scan-wide index.
        let mut db_rows: std::collections::HashMap<(u16, u64), u32> =
            std::collections::HashMap::new();
        for i in 0..self.object_entries.len() {
            let db_id = self.object_entries[i].db_id;
            let fp = self.entry_key(i).fingerprint();
            db_rows.insert((db_id, fp), (i + 1) as u32);
        }
        for ((db, key), entries) in staging {
            let db_id = self.intern_database(db.clone());
            let row_id = db_rows
                .get(&(db_id, key.fingerprint()))
                .copied()
                .unwrap_or(0);
            if row_id == 0 {
                tracing::warn!(
                    table = key.as_str(),
                    "dropping staged transitions: no matching table object (table .sql removed or renamed while _migrations/ remains?)"
                );
                continue;
            }
            let mut row_entries = Vec::with_capacity(entries.len());
            for (ord, sk) in entries {
                let Some(script_id) = self.script_key_index.get(&sk).copied() else {
                    tracing::warn!(
                        table = key.as_str(),
                        "dropping transition: script key not registered"
                    );
                    continue;
                };
                row_entries.push(TransitionEntry::new_staging(ord, script_id));
            }
            if !row_entries.is_empty() {
                self.transitions_by_row.insert(row_id, row_entries);
            }
        }
    }

    /// Appends a transition script to the staging map for `table_key` in `database`.
    pub fn push_transition_staging(
        &mut self,
        database: SharedStr,
        table_key: ObjectKey,
        ordinal: SharedStr,
        script_key: ScriptKey,
    ) -> Result<()> {
        // Reject two migration files claiming the same ordinal for one table IN
        // ONE DATABASE: the apply order would be ambiguous. Equal ordinals for
        // same-named tables in different catalog databases are legitimate.
        let staged = (database, table_key);
        if let Some(entries) = self.transitions_staging.get(&staged) {
            if entries.iter().any(|(ord, _)| *ord == ordinal) {
                return Err(Error::InvalidInput(format!(
                    "duplicate transition ordinal {} for {}/{}: each migration ordinal must be unique per table",
                    &*ordinal,
                    &*staged.0,
                    staged.1.as_str()
                )));
            }
        }
        self.transitions_staging
            .entry(staged)
            .or_default()
            .push((ordinal, script_key));
        Ok(())
    }
}
