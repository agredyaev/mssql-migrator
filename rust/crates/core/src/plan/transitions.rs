use std::collections::HashMap;

use crate::domain::{with_database_prefix, StrOff, Workspace};

pub fn paths_by_row(ws: &Workspace) -> HashMap<u32, Vec<StrOff>> {
    let arena = ws.layout_arena();
    let mut out = HashMap::new();
    for (&row_id, entries) in ws.transitions_by_row.iter() {
        let i = row_id as usize - 1;
        let db = ws
            .entry(i)
            .database_name(ws)
            .as_ref()
            .to_string();
        let db = db.as_str();
        let mut rows: Vec<_> = entries
            .iter()
            .filter_map(|e| {
                let script = ws.script(e.script_id);
                let ord = e
                    .staging_ord
                    .as_ref()
                    .map(|s| s.as_ref())
                    .unwrap_or_else(|| ws.str_at(e.ord_off));
                let path = with_database_prefix(db, script.path_str());
                Some((ord, path))
            })
            .collect();
        rows.sort_by(|a, b| a.0.cmp(b.0));
        let paths: Vec<StrOff> = rows
            .into_iter()
            .map(|(_, p)| StrOff::from_arena(arena, p.as_ref()))
            .collect();
        if !paths.is_empty() {
            out.insert(row_id, paths);
        }
    }
    out
}

pub fn rebuild_transition_path_cache(ws: &mut Workspace) {
    if ws.transitions_by_row.is_empty() || ws.layout_arena.is_none() {
        ws.transition_path_cache = None;
        ws.has_transition_paths_row.clear();
        return;
    }
    let cache = paths_by_row(ws);
    let n = ws.object_count();
    let mut flags = vec![0u8; n];
    for (row_id, paths) in cache.iter() {
        if paths.is_empty() {
            continue;
        }
        let i = *row_id as usize - 1;
        if i < n {
            flags[i] = 1;
        }
    }
    ws.has_transition_paths_row = flags;
    ws.transition_path_cache = Some(cache);
}
