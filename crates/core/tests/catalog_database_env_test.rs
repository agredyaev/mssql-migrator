use std::collections::HashMap;
use std::path::Path;

use migrator_core::config::{build_config, validate_config};

fn write_single_db_layout(root: &Path, db: &str) {
    std::fs::create_dir_all(root.join(format!("{db}/smoke/tables"))).expect("mkdir layout");
    std::fs::write(
        root.join(format!("{db}/smoke/tables/t1.sql")),
        "CREATE TABLE smoke.t1 (id INT NOT NULL);\n",
    )
    .expect("write sql");
}

fn write_multi_db_layout(root: &Path) {
    write_single_db_layout(root, "dactests");
    std::fs::create_dir_all(root.join("warehouse/reporting/views")).expect("mkdir warehouse");
    std::fs::write(
        root.join("warehouse/reporting/views/v1.sql"),
        "CREATE VIEW reporting.v1 AS SELECT 1 AS n;\n",
    )
    .expect("write view sql");
}

#[test]
fn validate_config_derives_database_from_sql_root_happy_path() {
    let dir = tempfile::tempdir().expect("tempdir");
    write_single_db_layout(dir.path(), "dactests");
    let mut env = HashMap::new();
    env.insert("RM_DB_SERVER".into(), "localhost".into());
    env.insert(
        "RM_SQL_ROOT".into(),
        dir.path().to_string_lossy().into_owned(),
    );
    env.insert("RM_DB_USER".into(), "sa".into());
    env.insert("RM_DB_PASSWORD".into(), "secret".into());
    let mut cfg = build_config(&env, false);
    validate_config(&mut cfg).expect("valid config");
    assert_eq!(cfg.database, "dactests");
}

#[test]
fn rm_db_database_env_does_not_override_catalog_negative_path() {
    let dir = tempfile::tempdir().expect("tempdir");
    write_single_db_layout(dir.path(), "dactests");
    let mut env = HashMap::new();
    env.insert("RM_DB_SERVER".into(), "localhost".into());
    env.insert(
        "RM_SQL_ROOT".into(),
        dir.path().to_string_lossy().into_owned(),
    );
    env.insert("RM_DB_DATABASE".into(), "wrongdb".into());
    env.insert("RM_DB_USER".into(), "sa".into());
    env.insert("RM_DB_PASSWORD".into(), "secret".into());
    let mut cfg = build_config(&env, false);
    validate_config(&mut cfg).expect("valid config");
    assert_ne!(cfg.database, "wrongdb");
    assert_eq!(cfg.database, "dactests");
}

#[test]
fn multi_database_layout_leaves_database_empty_edge_case() {
    let dir = tempfile::tempdir().expect("tempdir");
    write_multi_db_layout(dir.path());
    let mut env = HashMap::new();
    env.insert("RM_DB_SERVER".into(), "localhost".into());
    env.insert(
        "RM_SQL_ROOT".into(),
        dir.path().to_string_lossy().into_owned(),
    );
    env.insert("RM_DB_USER".into(), "sa".into());
    env.insert("RM_DB_PASSWORD".into(), "secret".into());
    let mut cfg = build_config(&env, false);
    validate_config(&mut cfg).expect("multi-db layout validates");
    assert!(
        cfg.database.is_empty(),
        "engine discovers per-db targets from RM_SQL_ROOT, not RM_DB_DATABASE"
    );
}

#[test]
fn rm_db_database_mismatch_with_sql_root_regression() {
    let dir = tempfile::tempdir().expect("tempdir");
    write_single_db_layout(dir.path(), "dactests");
    let mut env = HashMap::new();
    env.insert("RM_DB_SERVER".into(), "localhost".into());
    env.insert(
        "RM_SQL_ROOT".into(),
        dir.path().to_string_lossy().into_owned(),
    );
    env.insert("RM_DB_DATABASE".into(), "warehouse".into());
    env.insert("RM_DB_USER".into(), "sa".into());
    env.insert("RM_DB_PASSWORD".into(), "secret".into());
    let mut cfg = build_config(&env, false);
    validate_config(&mut cfg).expect("BG-014 regression: rmig must not honor RM_DB_DATABASE");
    assert_eq!(
        cfg.database, "dactests",
        "BG-014 regression: target DB comes from catalog layout, not RM_DB_DATABASE"
    );
}
