use migrator_core::config::validate_config;
use migrator_core::driver::{connect, mssql, select_auth_method};
use migrator_core::engine::{run_command, Command};
use migrator_core::Config;
use tiberius::AuthMethod;

fn integration_enabled() -> bool {
    std::env::var("RMIG_RUN_SQLSERVER_INTEGRATION")
        .map(|v| v == "1" || v.eq_ignore_ascii_case("true"))
        .unwrap_or(false)
}

#[test]
fn rm_db_auth_sql_mode_selects_sql_server_happy_path() {
    let mut cfg = Config::default();
    cfg.db_auth = "sql".into();
    cfg.user = "svc".into();
    cfg.password = "secret".into();
    assert!(matches!(
        select_auth_method(&cfg).expect("sql auth"),
        AuthMethod::SqlServer(_)
    ));
}

#[test]
fn rm_db_auth_unknown_value_fails_config_negative_path() {
    let mut cfg = Config::default();
    cfg.db_auth = "kerberos-only".into();
    let err = select_auth_method(&cfg).expect_err("unknown auth");
    assert!(
        err.to_string().contains("unsupported RM_DB_AUTH"),
        "unexpected error: {err}"
    );
}

#[test]
fn rm_db_auth_integrated_not_compiled_edge_case() {
    let mut cfg = Config::default();
    cfg.db_auth = "integrated".into();
    let err = select_auth_method(&cfg).expect_err("integrated auth not available");
    assert!(
        err.to_string().contains("integrated auth not compiled"),
        "unexpected error: {err}"
    );
}

#[test]
fn rm_db_auth_windows_not_compiled_edge_case() {
    let mut cfg = Config::default();
    cfg.db_auth = "windows".into();
    let err = select_auth_method(&cfg).expect_err("windows auth not available");
    assert!(
        err.to_string().contains("integrated auth not compiled"),
        "unexpected error: {err}"
    );
}

async fn recreate_database(database: &str) {
    let mut cfg = Config::default();
    cfg.server = std::env::var("RM_DB_SERVER").unwrap_or_else(|_| "localhost".into());
    cfg.port = std::env::var("RM_DB_PORT").unwrap_or_else(|_| "1433".into());
    cfg.user = std::env::var("RM_DB_USER").unwrap_or_else(|_| "sa".into());
    cfg.password =
        std::env::var("RM_DB_PASSWORD").unwrap_or_else(|_| "yourStrong(!)Password".into());
    cfg.database = "master".into();
    cfg.set_encrypt(false);
    cfg.set_trust_server_certificate(true);
    let mut master = connect(&cfg).await.expect("connect master");
    let escaped = database.replace('\'', "''");
    let sql = format!(
        "IF DB_ID(N'{escaped}') IS NOT NULL BEGIN ALTER DATABASE [{database}] SET SINGLE_USER WITH ROLLBACK IMMEDIATE; DROP DATABASE [{database}]; END; CREATE DATABASE [{database}];"
    );
    mssql::exec(&mut master.client, &sql)
        .await
        .expect("recreate database");
}

#[tokio::test]
async fn sql_auth_plan_still_connects_integration_happy_path() {
    if !integration_enabled() {
        return;
    }
    let tmp = tempfile::tempdir().expect("tempdir");
    let db = "dbauthsql";
    recreate_database(db).await;
    let root = tmp.path();
    std::fs::create_dir_all(root.join(format!("{db}/smoke/tables"))).expect("mkdir");
    std::fs::write(
        root.join(format!("{db}/smoke/tables/t1.sql")),
        "CREATE TABLE smoke.t1 (id INT NOT NULL);\n",
    )
    .expect("write sql");

    let mut cfg = Config::default();
    cfg.server = std::env::var("RM_DB_SERVER").unwrap_or_else(|_| "localhost".into());
    cfg.port = std::env::var("RM_DB_PORT").unwrap_or_else(|_| "1433".into());
    cfg.user = std::env::var("RM_DB_USER").unwrap_or_else(|_| "sa".into());
    cfg.password =
        std::env::var("RM_DB_PASSWORD").unwrap_or_else(|_| "yourStrong(!)Password".into());
    cfg.sql_root = root.to_str().expect("utf8").into();
    cfg.sql_base = cfg.sql_root.clone();
    cfg.db_auth = "sql".into();
    cfg.set_skip_git(true);
    cfg.set_encrypt(false);
    cfg.set_trust_server_certificate(true);
    validate_config(&mut cfg).expect("valid cfg");
    let out = run_command(Command::Plan, &cfg)
        .await
        .expect("sql auth plan");
    assert_eq!(out.exit_code, 0);
}
