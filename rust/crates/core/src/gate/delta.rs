use std::collections::HashSet;

use crate::domain::Workspace;

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
            keys.insert(k.as_str().to_string());
            continue;
        }
        if let Some(table_key) = trans_paths.get(&p) {
            keys.insert(table_key.clone());
            continue;
        }
        for (path, key) in &by_path {
            if path.ends_with(&p) || p.ends_with(path.as_str()) {
                keys.insert(key.as_str().to_string());
            }
        }
        for (path, table_key) in &trans_paths {
            if path.ends_with(&p) || p.ends_with(path.as_str()) {
                keys.insert(table_key.clone());
            }
        }
    }
    keys
}

pub fn expand_delta_closure(ws: &Workspace, mut delta: HashSet<String>) -> HashSet<String> {
    if delta.is_empty() {
        return delta;
    }
    loop {
        let mut added = 0usize;
        let n = ws.object_count();
        for i in 0..n {
            let obj = ws.entry(i);
            if !delta.contains(obj.key_str(ws)) {
                continue;
            }
            if obj.kind_part(ws) != "triggers" {
                continue;
            }
            let row_id = ws.row_id_at(i);
            if let Some(pref) = obj.parent_ref_for_row(ws, row_id) {
                if pref.parent_row_id > 0 {
                    let pk = ws.entry_key((pref.parent_row_id as usize) - 1).as_str();
                    if delta.insert(pk.to_string()) {
                        added += 1;
                    }
                }
            }
        }
        for (&row_id, entries) in ws.transitions_by_row.iter() {
            let table_key = ws.entry_key(row_id as usize - 1).as_str();
            if delta.contains(table_key) {
                continue;
            }
            for e in entries {
                let path = ws.script(e.script_id).path_str();
                if delta.contains(path) {
                    if delta.insert(table_key.to_string()) {
                        added += 1;
                    }
                    break;
                }
            }
        }
        if added == 0 {
            break;
        }
    }
    delta
}
