use std::fmt::Write;

use migrator_core::db::state::{catalog_object_from_key, CatalogState};
use migrator_core::db::ChecksumMap;
use migrator_core::domain::{ObjectKey, StrOff, Workspace};

use super::common::{
    bench_entry, bench_schema, checksum_for, finalize_bench_ws, insert_object_script, BenchBuild,
};

pub fn skip_heavy_workspace(n: usize) -> (Workspace, CatalogState, ChecksumMap) {
    let kinds = ["views", "procedures", "functions", "tables"];
    let mut build = BenchBuild::new(n, kinds.len());
    build.register_static_schema();
    for k in kinds {
        build.register(k);
    }
    let mut key_buf = String::with_capacity(48);
    let mut path_buf = String::with_capacity(64);
    for i in 0..n {
        build.register_skip_heavy_row(&mut key_buf, &mut path_buf, i, kinds[i % kinds.len()]);
    }
    let arena = build.finish();

    let mut ws = Workspace::default();
    let mut catalog = CatalogState::default();
    catalog.schemas.insert("schema".into());
    catalog.objects.reserve(n);
    let mut checksums = ChecksumMap::new();
    checksums.reserve(n);
    let (_schema_s, db_s) = bench_schema(&mut ws, &arena);
    let db_id = ws.intern_database(db_s);

    let mut entries = Vec::with_capacity(n);
    for i in 0..n {
        let kind = kinds[i % kinds.len()];
        key_buf.clear();
        let _ = write!(key_buf, "schema/{kind}/obj_{i}");
        let key_off = StrOff::from_arena(&arena, &key_buf);
        let key = ObjectKey::from(arena.shared_at(key_off.0, key_off.1));
        path_buf.clear();
        let _ = write!(path_buf, "testdb/schema/{kind}/obj_{i}.sql");
        let path_off = StrOff::from_arena(&arena, &path_buf);
        let path = arena.shared_at(path_off.0, path_off.1);
        let cs = checksum_for(i);
        let script_id = insert_object_script(&mut ws, path, cs);
        checksums.insert_key(&key, cs);
        let cat = catalog_object_from_key(&key);
        entries.push(bench_entry(key.clone(), script_id, cs, db_id));
        catalog.objects.insert(key, cat);
    }
    finalize_bench_ws(&mut ws, entries, arena);
    (ws, catalog, checksums)
}
