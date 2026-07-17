use super::*;
use crate::config::ConfigCold;
use std::sync::Arc;

fn make_cfg(server: &str, sql_root: &str, user: &str, password: &str, db_auth: &str) -> Config {
    Config {
        sql_root: sql_root.into(),
        cold: Arc::new(ConfigCold {
            server: server.into(),
            user: user.into(),
            password: password.into(),
            db_auth: db_auth.into(),
            ..Default::default()
        }),
        ..Config::default()
    }
}

#[test]
fn validate_config_sql_auth_happy_path() {
    let dir = tempfile::tempdir().expect("tempdir");
    std::fs::create_dir_all(dir.path().join("db1/smoke/tables")).expect("mkdir");
    std::fs::write(
        dir.path().join("db1/smoke/tables/t1.sql"),
        "CREATE TABLE smoke.t1 (id INT NOT NULL);\n",
    )
    .expect("write sql");
    let mut cfg = make_cfg(
        "localhost",
        &dir.path().to_string_lossy(),
        "svc",
        "secret",
        "",
    );
    validate_config(&mut cfg).expect("sql auth config should validate");
    assert_eq!(cfg.database, "db1");
}

#[test]
fn validate_config_rejects_non_numeric_port_negative_path() {
    let dir = tempfile::tempdir().expect("tempdir");
    std::fs::create_dir_all(dir.path().join("db1/smoke/tables")).expect("mkdir");
    std::fs::write(
        dir.path().join("db1/smoke/tables/t1.sql"),
        "CREATE TABLE smoke.t1 (id INT NOT NULL);\n",
    )
    .expect("write sql");
    let mut cfg = make_cfg(
        "localhost",
        &dir.path().to_string_lossy(),
        "svc",
        "secret",
        "",
    );
    cfg.port = "70000".into();
    let err = validate_config(&mut cfg).expect_err("out-of-range port should fail");
    assert!(
        err.to_string().contains("RM_DB_PORT"),
        "unexpected error: {err}"
    );
}

#[test]
fn validate_config_missing_sql_user_negative_path() {
    let mut cfg = make_cfg("localhost", "/tmp/sql", "", "secret", "");
    let err = validate_config(&mut cfg).expect_err("empty user should fail");
    assert!(
        err.to_string().contains("RM_DB_USER"),
        "unexpected error: {err}"
    );
}

#[test]
fn validate_config_missing_sql_password_negative_path() {
    let mut cfg = make_cfg("localhost", "/tmp/sql", "svc", "", "");
    let err = validate_config(&mut cfg).expect_err("empty password should fail");
    assert!(
        err.to_string().contains("RM_DB_PASSWORD"),
        "unexpected error: {err}"
    );
}

#[test]
fn validate_config_missing_sql_user_regression() {
    let mut cfg = make_cfg("localhost", "/tmp/sql", "", "", "");
    let err = validate_config(&mut cfg).expect_err("missing sql creds should fail fast");
    let msg = err.to_string();
    assert!(msg.contains("RM_DB_USER"), "unexpected error: {msg}");
    assert!(msg.contains("RM_DB_PASSWORD"), "unexpected error: {msg}");
}

/// Port zero parses as u16 but is not a reachable TCP destination; the
/// documented range starts at 1.
#[test]
fn validate_config_rejects_port_zero_regression() {
    let dir = tempfile::tempdir().expect("tempdir");
    std::fs::create_dir_all(dir.path().join("db1/smoke/tables")).expect("mkdir");
    std::fs::write(
        dir.path().join("db1/smoke/tables/t1.sql"),
        "CREATE TABLE smoke.t1 (id INT NOT NULL);\n",
    )
    .expect("write sql");
    let mut cfg = make_cfg(
        "localhost",
        &dir.path().to_string_lossy(),
        "svc",
        "secret",
        "",
    );
    cfg.port = "0".into();
    let err = validate_config(&mut cfg).expect_err("port zero should fail");
    assert!(
        err.to_string().contains("RM_DB_PORT"),
        "unexpected error: {err}"
    );
}
