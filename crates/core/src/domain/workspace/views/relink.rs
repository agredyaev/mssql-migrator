use super::super::Workspace;
use crate::domain::{ParentRef, TransitionEntry};

impl Workspace {
    pub(super) fn relink_transitions_from(&mut self, parent: &Workspace, database: &str) {
        for parent_object in parent
            .object_entries
            .iter()
            .filter(|object| parent.database_name(object.db_id) == database)
        {
            let child_row = self.key_index(&parent_object.key);
            if child_row == 0 {
                continue;
            }
            let transitions = parent_object
                .transitions
                .iter()
                .filter_map(|transition| {
                    let key = parent.script(transition.script_id).key();
                    self.script_key_index.get(&key).copied().map(|script_id| {
                        TransitionEntry::new(transition.ordinal.clone(), script_id)
                    })
                })
                .collect();
            self.object_entries[child_row as usize - 1].transitions = transitions;
        }
    }

    pub(super) fn relink_parents_from(&mut self, parent: &Workspace, database: &str) {
        for parent_object in parent
            .object_entries
            .iter()
            .filter(|object| parent.database_name(object.db_id) == database)
        {
            let Some(parent_ref) = parent_object.parent else {
                continue;
            };
            let Some(parent_table) = parent
                .object_entries
                .get(parent_ref.parent_row_id as usize - 1)
            else {
                continue;
            };
            let child_row = self.key_index(&parent_object.key);
            let parent_row = self.key_index(&parent_table.key);
            if child_row > 0 && parent_row > 0 {
                self.object_entries[child_row as usize - 1].parent = Some(ParentRef {
                    parent_row_id: parent_row,
                });
            }
        }
    }
}
