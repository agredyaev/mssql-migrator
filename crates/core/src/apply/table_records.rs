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
            let script = path_lookup_candidates(obj.database_name.as_ref(), path)
                .into_iter()
                .find_map(|key| ws.script_by_key(&ScriptKey::from_path(&key)));
            let cs = script
                .as_ref()
                .and_then(|s| s.checksum().copied())
                .unwrap_or(obj.checksum);
            // Provenance belongs to the transition script's own commit.
            let (hash, author, date) = script
                .as_ref()
                .map(|s| (s.git_hash(), s.git_author(), s.git_date()))
                .unwrap_or_else(|| {
                    (
                        obj.git_hash().into(),
                        obj.git_author().into(),
                        obj.git_date().into(),
                    )
                });
            audit::record_applied(
                path,
                &obj.kind,
                cs,
                hash.as_ref(),
                author.as_ref(),
                date.as_ref(),
                "migration",
            )
        })
        .collect()
}
