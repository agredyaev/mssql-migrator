use crate::audit::{self, HistoryRecord};
use crate::domain::{with_database_prefix, ObjectKey, Workspace};
use crate::export::PlannedObject;

/// A freshly created table's `.sql` already embodies all of its past transition
/// scripts; record them as applied so a later table change does not replay them.
pub(super) fn create_table_transition_records(
    ws: &Workspace,
    obj: &PlannedObject,
) -> Vec<HistoryRecord> {
    if obj.kind != "tables" {
        return Vec::new();
    }
    let row_id = ws.key_index(&ObjectKey::from_normalized(&obj.normalized_key));
    let Some(owner) = row_id
        .checked_sub(1)
        .and_then(|index| ws.object_entries.get(index as usize))
    else {
        return Vec::new();
    };
    owner
        .transitions
        .iter()
        .map(|transition| {
            let script = ws.script(transition.script_id);
            let path = with_database_prefix(&obj.database_name, script.path_str());
            let cs = script.checksum().copied().unwrap_or(obj.checksum);
            // Provenance belongs to the transition script's own commit.
            let hash = script.git_hash();
            let author = script.git_author();
            let date = script.git_date();
            audit::record_event(&path, cs, hash, author, date, "migration", "applied")
        })
        .collect()
}
