use std::sync::Arc;

use crate::config::{Config, ConfigCold};
use crate::driver::mssql_auth::select_auth_method;
use tiberius::AuthMethod;

fn make_cfg(db_auth: &str, user: &str, password: &str) -> Config {
    Config {
        cold: Arc::new(ConfigCold {
            db_auth: db_auth.into(),
            user: user.into(),
            password: password.into(),
            ..ConfigCold::default()
        }),
        ..Config::default()
    }
}

#[test]
fn select_auth_method_sql_auth_happy_path() {
    let cfg = make_cfg("sql", "svc", "secret");
    assert!(matches!(
        select_auth_method(&cfg).expect("sql auth"),
        AuthMethod::SqlServer(_)
    ));
}

#[test]
fn select_auth_method_unsupported_mode_negative_path() {
    let cfg = make_cfg("aad", "", "");
    let err = select_auth_method(&cfg).expect_err("unsupported auth");
    assert!(
        err.to_string().contains("unsupported RM_DB_AUTH"),
        "unexpected error: {err}"
    );
}

#[test]
fn select_auth_method_integrated_without_sql_credentials_edge_case() {
    let cfg = make_cfg("integrated", "", "");
    assert!(matches!(
        select_auth_method(&cfg).expect("integrated auth"),
        AuthMethod::Integrated
    ));
}

#[test]
fn select_auth_method_integrated_not_sql_server_regression() {
    let cfg = make_cfg("integrated", "ignored", "ignored");
    let auth = select_auth_method(&cfg).expect("BG-004 regression");
    assert!(
        !matches!(auth, AuthMethod::SqlServer(_)),
        "integrated auth must not use sql_server()"
    );
    assert!(matches!(auth, AuthMethod::Integrated));
}
