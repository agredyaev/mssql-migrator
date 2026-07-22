#![allow(missing_docs)]

use migrator_core::domain::{
    install_layout_arena, ObjectEntry, SchemaEntry, Script, ScriptKey, ScriptKind, SharedStr,
    StringArena, Workspace,
};

mod build;

pub use build::BenchBuild;

pub fn checksum_for(i: usize) -> [u8; 32] {
    let b = (i % 255 + 1) as u8;
    [b; 32]
}

pub fn bench_schema(ws: &mut Workspace, arena: &StringArena) -> (SharedStr, SharedStr) {
    let schema_s = arena.get("schema");
    let db_s = arena.get("testdb");
    ws.schemas.push(SchemaEntry {
        database: db_s.clone(),
        name: schema_s.clone(),
        normalized: schema_s.clone(),
    });
    (schema_s, db_s)
}

pub fn insert_object_script(ws: &mut Workspace, path: SharedStr, cs: [u8; 32]) -> u32 {
    ws.insert_script(Script {
        key: ScriptKey::from(path.clone()),
        kind: ScriptKind::Object,
        abs_path: path,
        checksum: Some(cs),
    })
}

pub fn bench_entry(
    key: migrator_core::domain::ObjectKey,
    script_id: u32,
    cs: [u8; 32],
    db_id: u16,
) -> (migrator_core::domain::ObjectKey, ObjectEntry) {
    ObjectEntry::with_staging_key(key, script_id, cs, true, db_id)
}

pub fn finalize_bench_ws(
    ws: &mut Workspace,
    pairs: Vec<(migrator_core::domain::ObjectKey, ObjectEntry)>,
    arena: StringArena,
) {
    ws.adopt_dense_entries(pairs);
    install_layout_arena(ws, arena);
    migrator_core::domain::rebuild_path_caches(ws);
}
