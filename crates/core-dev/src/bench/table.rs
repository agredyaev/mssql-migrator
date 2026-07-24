use migrator_core::db::state::{catalog_object_from_key, CatalogState};
use migrator_core::db::ChecksumMap;
use migrator_core::domain::{ObjectKey, Script, ScriptKey, ScriptKind, Workspace};

use super::common::{
    bench_entry, bench_schema, checksum_for, finalize_bench_ws, insert_object_script,
};

pub fn table_heavy_workspace(n_tables: usize) -> (Workspace, CatalogState, ChecksumMap) {
    let mut ws = Workspace::default();
    let mut catalog = CatalogState::default();
    catalog.schemas.insert("schema".into());
    let mut checksums = ChecksumMap::new();
    let (_schema_s, db_s) = bench_schema(&mut ws);
    let db_id = ws.intern_database(db_s);

    let mut entries = Vec::with_capacity(n_tables);
    for i in 0..n_tables {
        let key = ObjectKey::new("schema", "tables", &format!("t_{i}"));
        let table_path = format!("testdb/schema/tables/t_{i}.sql");
        let file_cs = checksum_for(i + 1);
        let prior_cs = checksum_for(i);
        let script_id = insert_object_script(&mut ws, table_path, file_cs);
        entries.push(bench_entry(key.clone(), script_id, file_cs, db_id));
        for ord in ["001", "002"] {
            let trans_path = format!(
                "testdb/schema/tables/t_{i}/_migrations/{}_{}.sql",
                ord,
                key.name_part()
            );
            let tsk = ScriptKey::from(trans_path.clone());
            ws.insert_script(Script {
                key: tsk.clone(),
                kind: ScriptKind::Transition,
                abs_path: trans_path,
                checksum: Some([ord.as_bytes()[0]; 32]),
            });
            ws.push_transition_staging("testdb".into(), key.clone(), ord.to_owned(), tsk)
                .expect("bench fixture ordinals are unique per table");
        }
        catalog
            .objects
            .insert(key.clone(), catalog_object_from_key(&key));
        checksums.insert_key(&key, prior_cs);
    }
    finalize_bench_ws(&mut ws, entries);
    (ws, catalog, checksums)
}
