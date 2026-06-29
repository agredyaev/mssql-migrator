use super::super::Workspace;
use crate::domain::object::ParentRef;
use crate::domain::share;
use crate::domain::transition::TransitionEntry;

impl Workspace {
    /// Remap transition scripts from a full-layout parent (row ids + script ids differ per subset).
    pub(super) fn relink_transitions_from(&mut self, parent: &Workspace, db: &str) {
        self.transitions_by_row.clear();
        for (&parent_row_id, entries) in parent.transitions_by_row.iter() {
            let pi = parent_row_id as usize;
            if pi == 0 || pi > parent.object_entries.len() {
                continue;
            }
            let idx = pi - 1;
            if parent.database_name(parent.entry(idx).db_id).as_ref() != db {
                continue;
            }
            let table_key = parent.entry_key(idx);
            let child_row_id = self.key_index(table_key);
            if child_row_id == 0 {
                continue;
            }
            let mut mapped = Vec::with_capacity(entries.len());
            for e in entries {
                let sk = parent.script(e.script_id).key();
                let Some(child_script_id) = self.script_key_index.get(&sk).copied() else {
                    continue;
                };
                let ord_name = e
                    .staging_ord
                    .as_ref()
                    .map(|o| o.as_ref())
                    .unwrap_or_else(|| parent.str_at(e.ord_off));
                mapped.push(TransitionEntry::new_staging(
                    share(ord_name),
                    child_script_id,
                ));
            }
            if !mapped.is_empty() {
                self.transitions_by_row.insert(child_row_id, mapped);
            }
        }
    }

    pub(super) fn relink_parents_from(&mut self, parent: &Workspace, db: &str) {
        self.parent_by_row.clear();
        for (&parent_child_row, pref) in parent.parent_by_row.iter() {
            let ci = parent_child_row as usize;
            if ci == 0 || ci > parent.object_entries.len() {
                continue;
            }
            let cidx = ci - 1;
            if parent.database_name(parent.entry(cidx).db_id).as_ref() != db {
                continue;
            }
            let child_key = parent.entry_key(cidx);
            let child_row_id = self.key_index(child_key);
            if child_row_id == 0 {
                continue;
            }
            let pi = pref.parent_row_id as usize;
            if pi == 0 || pi > parent.object_entries.len() {
                continue;
            }
            let parent_table_key = parent.entry_key(pi - 1);
            let child_parent_row = self.key_index(parent_table_key);
            if child_parent_row == 0 {
                continue;
            }
            self.parent_by_row.insert(
                child_row_id,
                ParentRef {
                    parent_row_id: child_parent_row,
                },
            );
        }
    }
}
