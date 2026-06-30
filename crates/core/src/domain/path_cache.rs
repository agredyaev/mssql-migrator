use std::collections::HashMap;

use super::{object_path_for_entry, with_database_prefix, StrOff, Workspace};

/// Builds a map from row ID to ordered transition path offsets for all rows in `ws`.
pub fn paths_by_row(ws: &Workspace) -> HashMap<u32, Vec<StrOff>> {
    let arena = ws.layout_arena();
    let mut out = HashMap::new();
    for (&row_id, entries) in ws.transitions_by_row.iter() {
        let i = row_id as usize - 1;
        let db = ws.entry(i).database_name(ws).as_ref().to_string();
        let db = db.as_str();
        let mut rows: Vec<_> = entries
            .iter()
            .map(|e| {
                let script = ws.script(e.script_id);
                let ord = e
                    .staging_ord
                    .as_ref()
                    .map(|s| s.as_ref())
                    .unwrap_or_else(|| ws.str_at(e.ord_off));
                let path = with_database_prefix(db, script.path_str());
                (ord, path)
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

/// Rebuilds the workspace's transition-path cache and per-row presence flags.
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

/// Rebuilds the workspace's object-path cache from the current entries.
pub fn rebuild_object_path_cache(ws: &mut Workspace) {
    let n = ws.object_entries.len();
    if n == 0 || ws.layout_arena.is_none() {
        ws.object_path_cache = None;
        return;
    }
    let arena = ws.layout_arena();
    let mut paths = Vec::with_capacity(n);
    for obj in &ws.object_entries {
        let script = ws.script(obj.script_id);
        let path =
            object_path_for_entry(ws.database_name(obj.db_id).as_ref(), script.key().shared());
        paths.push(StrOff::from_arena(arena, path.as_ref()));
    }
    ws.object_path_cache = Some(paths);
}

/// Rebuilds both the transition-path cache and the object-path cache.
pub fn rebuild_path_caches(ws: &mut Workspace) {
    rebuild_transition_path_cache(ws);
    rebuild_object_path_cache(ws);
}

/// Rebuilds whichever path caches are stale, if any.
pub fn ensure_path_caches(ws: &mut Workspace) {
    if !ws.object_entries.is_empty() && ws.object_path_cache.is_none() {
        rebuild_path_caches(ws);
    } else if !ws.transitions_by_row.is_empty() && ws.transition_path_cache.is_none() {
        rebuild_transition_path_cache(ws);
    }
}
