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
        ws.for_each_entry(|obj| {
            if !delta.contains(obj.key.as_str()) {
                return;
            }
            if obj.kind.as_ref() == "triggers" {
                if let Some(pk) = &obj.parent_key {
                    if delta.insert(pk.as_str().to_string()) {
                        added += 1;
                    }
                }
            }
        });
        for table_key in ws.transitions_by_table.keys() {
            if delta.contains(table_key.as_str()) {
                continue;
            }
            for (_, sk) in &ws.transitions_by_table[table_key] {
                let path = ws.scripts.get(sk).map(|s| s.key.as_str()).unwrap_or("");
                if delta.contains(path) {
                    if delta.insert(table_key.as_str().to_string()) {
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
