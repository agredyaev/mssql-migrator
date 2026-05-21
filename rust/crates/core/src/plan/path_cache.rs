use crate::domain::{object_path_for_entry, StrOff, Workspace};

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
        let path = object_path_for_entry(
            ws.database_name(obj.db_id).as_ref(),
            script.key().shared(),
        );
        paths.push(StrOff::from_arena(arena, path.as_ref()));
    }
    ws.object_path_cache = Some(paths);
}

pub fn rebuild_path_caches(ws: &mut Workspace) {
    super::transitions::rebuild_transition_path_cache(ws);
    rebuild_object_path_cache(ws);
}

pub fn ensure_path_caches(ws: &mut Workspace) {
    if !ws.object_entries.is_empty() && ws.object_path_cache.is_none() {
        rebuild_path_caches(ws);
    } else if !ws.transitions_by_row.is_empty() && ws.transition_path_cache.is_none() {
        super::transitions::rebuild_transition_path_cache(ws);
    }
}

#[cfg(test)]
mod tests {
    use crate::domain::{intern_workspace_strings, share, ObjectEntry, ObjectKey, Script, ScriptKind, ScriptKey, Workspace};

    use super::rebuild_object_path_cache;

    #[test]
    fn object_path_cache_len_matches_entries() {
        let mut ws = Workspace::default();
        let path = share("testdb/schema/tables/t1.sql");
        let sid = ws.insert_script(Script {
            key: ScriptKey::from(path.clone()),
            kind: ScriptKind::Object,
            abs_path: path.clone(),
            checksum: None,
            scaffold: false,
        });
        let db_id = ws.intern_database(share("testdb"));
        let key = ObjectKey::from(share("schema/tables/t1"));
        ws.adopt_dense_entries(vec![ObjectEntry::with_staging_key(
            key, sid, [1; 32], true, db_id,
        )]);
        intern_workspace_strings(&mut ws);
        rebuild_object_path_cache(&mut ws);
        let cache = ws.object_path_cache.as_ref().expect("cache");
        assert_eq!(cache.len(), ws.object_count());
        assert_eq!(ws.str_at(cache[0]), "testdb/schema/tables/t1.sql");
    }
}
