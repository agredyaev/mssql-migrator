use crate::db::catalog::build_catalog_sql_batch;

#[test]
fn build_catalog_sql_includes_sys_objects_for_tables_only() {
    let sql = build_catalog_sql_batch(&["tables"], false);
    assert!(
        sql.contains("sys_object_rows"),
        "tables-only inspect must query sys.objects"
    );
}
