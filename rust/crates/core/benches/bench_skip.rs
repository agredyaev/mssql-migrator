use migrator_core::db::state::{catalog_object, CatalogState};
use migrator_core::db::ChecksumMap;
use migrator_core::domain::{ObjectKey, StringInterner, Workspace};

use super::bench_common::{
    bench_entry, bench_schema, checksum_for, finalize_bench_ws, insert_object_script,
};

pub fn skip_heavy_workspace(n: usize) -> (Workspace, CatalogState, ChecksumMap) {
    let mut ws = Workspace::default();
    let mut catalog = CatalogState::default();
    catalog.schemas.insert("schema".into());
    let mut checksums = ChecksumMap::new();
    let mut interner = StringInterner::with_capacity(n / 4 + 8);
    let (schema_s, db_s) = bench_schema(&mut ws, &mut interner);
    let db_id = ws.intern_database(db_s.clone());
    let kinds = ["views", "procedures", "functions", "tables"];
    let kind_shared: Vec<_> = kinds.iter().map(|k| interner.intern(k)).collect();
    let mut entries = Vec::with_capacity(n);
    for i in 0..n {
        let kind_s = kind_shared[i % kinds.len()].clone();
        let name = interner.intern(&format!("obj_{i}"));
        let key = ObjectKey::from(interner.intern(&format!(
            "schema/{}/{}",
            kind_s.as_ref(),
            name.as_ref()
        )));
        let path = interner.intern(&format!(
            "testdb/schema/{}/{}.sql",
            kind_s.as_ref(),
            name.as_ref()
        ));
        let cs = checksum_for(i);
        let script_id = insert_object_script(&mut ws, path, &schema_s, &kind_s, &name, cs);
        entries.push(bench_entry(key.clone(), script_id, cs, db_id));
        catalog.objects.insert(
            key.clone(),
            catalog_object("schema", kinds[i % kinds.len()], &format!("obj_{i}"), None),
        );
        checksums.insert_key(&key, cs);
    }
    finalize_bench_ws(&mut ws, entries, &interner);
    (ws, catalog, checksums)
}
