use crate::config::catalog::discover_catalog_databases;
use std::fs;

#[test]
fn discover_databases_from_layout() {
    let base = std::env::temp_dir().join(format!("rmig_catalog_{}", std::process::id()));
    let _ = fs::remove_dir_all(&base);
    fs::create_dir_all(base.join("dactests/smoke/tables")).unwrap();
    fs::create_dir_all(base.join("otherdb/reporting/views")).unwrap();
    let dbs = discover_catalog_databases(base.to_str().unwrap()).unwrap();
    assert_eq!(dbs, vec!["dactests", "otherdb"]);
    let _ = fs::remove_dir_all(&base);
}
