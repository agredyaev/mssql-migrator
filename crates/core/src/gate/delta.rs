use std::collections::HashSet;

use crate::domain::Workspace;

/// Snapshot-object identity for gate deltas: `database/normalized_key`, the
/// database taken from the script path's first component (matches
/// `PlanSnapshot` keys, which are database-qualified).
fn qualified_key(path: &str, key: &str) -> String {
    match path.split('/').next() {
        Some(db) if !db.is_empty() => format!("{db}/{key}"),
        _ => key.to_string(),
    }
}

/// Returns the set of snapshot keys that match any of the changed file paths.
pub fn keys_for_changed_paths(ws: &Workspace, changed_paths: &[String]) -> HashSet<String> {
    let mut keys = HashSet::new();
    if changed_paths.is_empty() {
        return keys;
    }
    let by_path = ws.objects_by_path();
    let trans_paths = ws.transition_paths_by_script();
    for raw in changed_paths {
        let p = raw.trim().replace('\\', "/");
        if p.is_empty() {
            continue;
        }
        if let Some(k) = by_path.get(&p) {
            keys.insert(qualified_key(&p, k.as_str()));
            continue;
        }
        if let Some(table_key) = trans_paths.get(&p) {
            keys.insert(qualified_key(&p, table_key));
            continue;
        }
        for (path, key) in &by_path {
            if path.ends_with(&p) || p.ends_with(path.as_str()) {
                keys.insert(qualified_key(path, key.as_str()));
            }
        }
        for (path, table_key) in &trans_paths {
            if path.ends_with(&p) || p.ends_with(path.as_str()) {
                keys.insert(qualified_key(path, table_key));
            }
        }
    }
    keys
}

/// Expands `delta` to include parent tables of matched triggers. Changed transition
/// scripts already enter as their table's qualified key via `keys_for_changed_paths`.
pub fn expand_delta_closure(ws: &Workspace, mut delta: HashSet<String>) -> HashSet<String> {
    if delta.is_empty() {
        return delta;
    }
    loop {
        let mut added = 0usize;
        let n = ws.object_count();
        for i in 0..n {
            let obj = ws.entry(i);
            let db = ws.database_name(obj.db_id);
            if !delta.contains(&format!("{db}/{}", obj.key.as_str())) {
                continue;
            }
            if obj.key.kind_part() != "triggers" {
                continue;
            }
            if let Some(pref) = obj.parent {
                if pref.parent_row_id > 0 {
                    let pi = (pref.parent_row_id as usize) - 1;
                    let pk = ws.entry_key(pi).as_str();
                    let pdb = ws.database_name(ws.entry(pi).db_id);
                    if delta.insert(format!("{pdb}/{pk}")) {
                        added += 1;
                    }
                }
            }
        }
        if added == 0 {
            break;
        }
    }
    delta
}
