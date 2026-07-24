#![allow(missing_docs)]

use migrator_core::domain::{ObjectEntry, SchemaEntry, Script, ScriptKey, ScriptKind, Workspace};

pub fn checksum_for(i: usize) -> [u8; 32] {
    let b = (i % 255 + 1) as u8;
    [b; 32]
}

pub fn bench_schema(ws: &mut Workspace) -> (String, String) {
    let schema_s = "schema".to_owned();
    let db_s = "testdb".to_owned();
    ws.schemas.push(SchemaEntry {
        database: db_s.clone(),
        name: schema_s.clone(),
        normalized: schema_s.clone(),
    });
    (schema_s, db_s)
}

pub fn insert_object_script(ws: &mut Workspace, path: String, cs: [u8; 32]) -> u32 {
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
) -> ObjectEntry {
    ObjectEntry::new(key, script_id, cs, true, db_id)
}

pub fn finalize_bench_ws(ws: &mut Workspace, objects: Vec<ObjectEntry>) {
    ws.adopt_dense_entries(objects);
}
