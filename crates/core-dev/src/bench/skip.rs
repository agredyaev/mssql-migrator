use migrator_core::db::state::{catalog_object_from_key, CatalogState};
use migrator_core::db::ChecksumMap;
use migrator_core::domain::{ObjectKey, Workspace};

use super::common::{
    bench_entry, bench_schema, checksum_for, finalize_bench_ws, insert_object_script,
};

pub fn skip_heavy_workspace(n: usize) -> (Workspace, CatalogState, ChecksumMap) {
    let kinds = ["views", "procedures", "functions", "tables"];
    let mut ws = Workspace::default();
    let mut catalog = CatalogState::default();
    catalog.schemas.insert("schema".into());
    catalog.objects.reserve(n);
    let mut checksums = ChecksumMap::new();
    checksums.reserve(n);
    let (_schema_s, db_s) = bench_schema(&mut ws);
    let db_id = ws.intern_database(db_s);

    let mut entries = Vec::with_capacity(n);
    for i in 0..n {
        let kind = kinds[i % kinds.len()];
        let key = ObjectKey::new("schema", kind, &format!("obj_{i}"));
        let path = format!("testdb/schema/{kind}/obj_{i}.sql");
        let cs = checksum_for(i);
        let script_id = insert_object_script(&mut ws, path, cs);
        checksums.insert_key(&key, cs);
        let cat = catalog_object_from_key(&key);
        entries.push(bench_entry(key.clone(), script_id, cs, db_id));
        catalog.objects.insert(key, cat);
    }
    finalize_bench_ws(&mut ws, entries);
    (ws, catalog, checksums)
}
