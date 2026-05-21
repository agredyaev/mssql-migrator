use migrator_core::db::state::{catalog_object, CatalogState};
use migrator_core::db::ChecksumMap;
use migrator_core::domain::{
    empty_str, share, ObjectKey, Script, ScriptKind, ScriptKey, StringInterner, Workspace,
};

use super::bench_common::{
    bench_entry, bench_schema, checksum_for, finalize_bench_ws, insert_object_script,
};

pub fn table_heavy_workspace(n_tables: usize) -> (Workspace, CatalogState, ChecksumMap) {
    let mut ws = Workspace::default();
    let mut catalog = CatalogState::default();
    catalog.schemas.insert("schema".into());
    let mut checksums = ChecksumMap::new();
    let mut interner = StringInterner::with_capacity(n_tables * 3 + 8);
    let (schema_s, db_s) = bench_schema(&mut ws, &mut interner);
    let db_id = ws.intern_database(db_s.clone());
    let kind_s = interner.intern("tables");
    let mut entries = Vec::with_capacity(n_tables);
    for i in 0..n_tables {
        let name = interner.intern(&format!("t_{i}"));
        let key = ObjectKey::from(interner.intern(&format!("schema/tables/{}", name.as_ref())));
        let table_path = interner.intern(&format!("testdb/schema/tables/{}.sql", name.as_ref()));
        let file_cs = checksum_for(i + 1);
        let prior_cs = checksum_for(i);
        let script_id =
            insert_object_script(&mut ws, table_path, &schema_s, &kind_s, &name, file_cs);
        entries.push(bench_entry(key.clone(), script_id, file_cs, db_id));
        for ord in ["001", "002"] {
            let trans_path = interner.intern(&format!(
                "testdb/schema/tables/{}/_migrations/{}_{}.sql",
                name.as_ref(),
                ord,
                name.as_ref()
            ));
            let tsk = ScriptKey::from(trans_path.clone());
            ws.insert_script(Script {
                key: tsk.clone(),
                kind: ScriptKind::Transition,
                abs_path: trans_path,
                checksum: Some([ord.as_bytes()[0]; 32]),
                scaffold: false,
            });
            ws.push_transition_staging(key.clone(), share(ord), tsk);
        }
        catalog.objects.insert(
            key.clone(),
            catalog_object("schema", "tables", &format!("t_{i}"), None),
        );
        checksums.insert_key(&key, prior_cs);
    }
    finalize_bench_ws(&mut ws, entries, &interner);
    (ws, catalog, checksums)
}
