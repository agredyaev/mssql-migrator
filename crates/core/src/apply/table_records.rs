use crate::audit::{self, HistoryRecord};
use crate::domain::{path_lookup_candidates, ObjectKey, ScriptKey, Workspace};
use crate::export::PlannedObject;

/// A freshly created table's `.sql` already embodies all of its past transition
/// scripts; record them as applied so a later table change does not replay them.
pub(super) fn create_table_transition_records(
    ws: &Workspace,
    obj: &PlannedObject,
) -> Vec<HistoryRecord> {
    if obj.kind.as_ref() != "tables" {
        return Vec::new();
    }
    let row_id = ws.key_index(&ObjectKey::from_normalized(obj.normalized_key.as_ref()));
    let Some(paths) = ws
        .transition_path_cache
        .as_ref()
        .and_then(|m| m.get(&row_id))
    else {
        return Vec::new();
    };
    paths
        .iter()
        .map(|off| {
            let path = ws.str_at(*off);
            let cs = path_lookup_candidates(obj.database_name.as_ref(), path)
                .into_iter()
                .find_map(|key| {
                    ws.script_by_key(&ScriptKey::from_path(&key))
                        .and_then(|s| s.checksum().copied())
                })
                .unwrap_or(obj.checksum);
            audit::record_applied(
                path,
                &obj.kind,
                cs,
                obj.git_hash(),
                obj.git_author(),
                obj.git_date(),
                "migration",
            )
        })
        .collect()
}
