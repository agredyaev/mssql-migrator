use migrator_core::domain::{
    share, ObjectEntry, SchemaEntry, Script, ScriptKind, ScriptKey, SharedStr, StringInterner,
    Workspace,
};

pub(crate) fn checksum_for(i: usize) -> [u8; 32] {
    let b = (i % 255 + 1) as u8;
    [b; 32]
}

pub(crate) fn bench_schema(ws: &mut Workspace, interner: &mut StringInterner) -> (SharedStr, SharedStr) {
    let schema_s = interner.intern("schema");
    let db_s = interner.intern("testdb");
    ws.schemas.push(SchemaEntry {
        database: share("testdb"),
        name: share("schema"),
        normalized: share("schema"),
    });
    (schema_s, db_s)
}

pub(crate) fn insert_object_script(
    ws: &mut Workspace,
    path: SharedStr,
    _schema_s: &SharedStr,
    _kind_s: &SharedStr,
    _name: &SharedStr,
    cs: [u8; 32],
) -> u32 {
    ws.insert_script(Script {
        key: ScriptKey::from(path.clone()),
        kind: ScriptKind::Object,
        abs_path: path,
        checksum: Some(cs),
        scaffold: false,
    })
}

pub(crate) fn bench_entry(
    key: migrator_core::domain::ObjectKey,
    script_id: u32,
    cs: [u8; 32],
    db_id: u16,
) -> ObjectEntry {
    ObjectEntry::with_staging_key(key, script_id, cs, true, db_id)
}

pub(crate) fn finalize_bench_ws(ws: &mut Workspace, entries: Vec<ObjectEntry>, interner: &StringInterner) {
    ws.string_arena_bytes = interner.byte_len();
    ws.string_arena_unique = interner.unique_count();
    ws.adopt_dense_entries(entries);
    migrator_core::domain::intern_workspace_strings(ws);
    migrator_core::plan::rebuild_path_caches(ws);
}
