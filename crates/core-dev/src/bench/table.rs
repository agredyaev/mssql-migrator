use std::fmt::Write;

use migrator_core::db::state::{catalog_object_from_key, CatalogState};
use migrator_core::db::ChecksumMap;
use migrator_core::domain::{ObjectKey, Script, ScriptKey, ScriptKind, StrOff, Workspace};

use super::common::{
    bench_entry, bench_schema, checksum_for, finalize_bench_ws, insert_object_script, BenchBuild,
};

pub fn table_heavy_workspace(n_tables: usize) -> (Workspace, CatalogState, ChecksumMap) {
    let mut build = BenchBuild::new(n_tables, n_tables * 2 + 4);
    build.register_static_schema();
    build.register("tables");
    build.register("001");
    build.register("002");
    let mut key_buf = String::with_capacity(48);
    let mut path_buf = String::with_capacity(96);
    for i in 0..n_tables {
        build.register_table_row(&mut key_buf, &mut path_buf, i);
        build.register_table_transition(&mut path_buf, i, "001");
        build.register_table_transition(&mut path_buf, i, "002");
    }
    let arena = build.finish();

    let mut ws = Workspace::default();
    let mut catalog = CatalogState::default();
    catalog.schemas.insert("schema".into());
    let mut checksums = ChecksumMap::new();
    let (_schema_s, db_s) = bench_schema(&mut ws, &arena);
    let db_id = ws.intern_database(db_s);
    let ord_001 = arena.get("001");
    let ord_002 = arena.get("002");

    let mut entries = Vec::with_capacity(n_tables);
    for i in 0..n_tables {
        key_buf.clear();
        let _ = write!(key_buf, "schema/tables/t_{i}");
        let key_off = StrOff::from_arena(&arena, &key_buf);
        let key = ObjectKey::from(arena.shared_at(key_off.0, key_off.1));
        path_buf.clear();
        let _ = write!(path_buf, "testdb/schema/tables/t_{i}.sql");
        let path_off = StrOff::from_arena(&arena, &path_buf);
        let table_path = arena.shared_at(path_off.0, path_off.1);
        let file_cs = checksum_for(i + 1);
        let prior_cs = checksum_for(i);
        let script_id = insert_object_script(&mut ws, table_path, file_cs);
        entries.push(bench_entry(key.clone(), script_id, file_cs, db_id));
        for ord in [&ord_001, &ord_002] {
            path_buf.clear();
            let _ = write!(
                path_buf,
                "testdb/schema/tables/t_{i}/_migrations/{}_{}.sql",
                ord.as_ref(),
                key.name_part()
            );
            let trans_off = StrOff::from_arena(&arena, &path_buf);
            let trans_path = arena.shared_at(trans_off.0, trans_off.1);
            let tsk = ScriptKey::from(trans_path.clone());
            ws.insert_script(Script {
                key: tsk.clone(),
                kind: ScriptKind::Transition,
                abs_path: trans_path,
                checksum: Some([ord.as_ref().as_bytes()[0]; 32]),
                scaffold: false,
            });
            ws.push_transition_staging(key.clone(), ord.clone(), tsk)
                .expect("bench fixture ordinals are unique per table");
        }
        catalog
            .objects
            .insert(key.clone(), catalog_object_from_key(&key));
        checksums.insert_key(&key, prior_cs);
    }
    finalize_bench_ws(&mut ws, entries, arena);
    (ws, catalog, checksums)
}
