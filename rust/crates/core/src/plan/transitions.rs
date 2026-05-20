use std::collections::HashMap;

use crate::domain::{ObjectKey, SharedStr, Workspace};

pub fn paths_by_table(ws: &Workspace) -> HashMap<ObjectKey, Vec<SharedStr>> {
    let mut out = HashMap::new();
    for (key, entries) in &ws.transitions_by_table {
        let mut rows: Vec<_> = entries
            .iter()
            .filter_map(|(ord, sk)| {
                ws.scripts
                    .get(sk)
                    .map(|s| (ord.clone(), s.key.shared()))
            })
            .collect();
        rows.sort_by(|a, b| a.0.as_ref().cmp(b.0.as_ref()));
        let paths: Vec<SharedStr> = rows.into_iter().map(|(_, p)| p).collect();
        if !paths.is_empty() {
            out.insert(key.clone(), paths);
        }
    }
    out
}

pub fn rebuild_transition_path_cache(ws: &mut Workspace) {
    ws.transition_path_cache = if ws.transitions_by_table.is_empty() {
        None
    } else {
        Some(paths_by_table(ws))
    };
}
