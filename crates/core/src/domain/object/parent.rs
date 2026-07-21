use super::super::shared::{empty_str, SharedStr};
use super::{ObjectEntry, ParentRef};

impl ObjectEntry {
    /// Parent table row for trigger at `child_row_id` (1-based), if recorded at catalog apply.
    pub fn parent_ref_for_row<'a>(
        &'a self,
        ws: &'a super::super::Workspace,
        child_row_id: u32,
    ) -> Option<&'a ParentRef> {
        ws.parent_by_row.get(&child_row_id)
    }

    /// Materialize for JSON export only.
    pub fn parent_name(ws: &super::super::Workspace, child_row_id: u32) -> SharedStr {
        let Some(pref) = ws.parent_by_row.get(&child_row_id) else {
            return empty_str();
        };
        if pref.parent_row_id == 0 {
            return empty_str();
        }
        let pi = (pref.parent_row_id as usize) - 1;
        ws.entry(pi).name_shared(ws, pi)
    }
}
