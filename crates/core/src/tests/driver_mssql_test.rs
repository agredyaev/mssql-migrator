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

#[tokio::test]
async fn session_init_step_is_bounded_regression() {
    let err = super::with_connect_timeout(
        std::time::Duration::from_millis(1),
        "sql.example:1433",
        "session init",
        std::future::pending::<()>(),
    )
    .await
    .expect_err("stalled session init must time out");
    let msg = err.to_string();
    assert!(msg.contains("session init timed out"), "{msg}");
}
