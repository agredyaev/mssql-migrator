use migrator_core::config::validate_config;
use migrator_core::driver::{connect, mssql};
use migrator_core::engine::{run_command, Command};
use migrator_core::error::{Error, EXIT_CONFIG, EXIT_CONN};
use migrator_core::session::connect_session_or_direct;
use migrator_core::Config;

fn integration_enabled() -> bool {
    std::env::var("RMIG_RUN_SQLSERVER_INTEGRATION")
        .map(|v| v == "1" || v.eq_ignore_ascii_case("true"))
        .unwrap_or(false)
}

fn parity_cfg(sql_root: &str) -> Config {
    let mut cfg = Config {
        server: std::env::var("RM_DB_SERVER").unwrap_or_else(|_| "localhost".into()),
        port: std::env::var("RM_DB_PORT").unwrap_or_else(|_| "1433".into()),
        user: std::env::var("RM_DB_USER").unwrap_or_else(|_| "sa".into()),
        password: std::env::var("RM_DB_PASSWORD")
            .unwrap_or_else(|_| "yourStrong(!)Password".into()),
        sql_root: sql_root.into(),
        sql_base: sql_root.into(),
        skip_git: true,
        encrypt: false,
        trust_server_certificate: true,
        ..Default::default()
    };
    validate_config(&mut cfg).expect("valid parity cfg");
    cfg
}

#[tokio::test]
async fn empty_session_socket_uses_direct_connect_edge_case() {
    let cfg = Config {
        server: "127.0.0.1".into(),
        port: "1".into(),
        user: "sa".into(),
        password: "x".into(),
        session_socket: String::new(),
        ..Default::default()
    };
    let err: Error = connect_session_or_direct(&cfg)
        .await
        .err()
        .expect("direct connect should fail against closed port");
    assert_ne!(
        err.exit_code(),
        EXIT_CONFIG,
        "empty RMIG_SESSION must not fail as config"
    );
}

#[tokio::test]
async fn missing_daemon_socket_falls_back_to_direct_negative_path() {
    let cfg = Config {
        session_socket: format!("/tmp/rmig-missing-{}.sock", std::process::id()),
        server: "127.0.0.1".into(),
        port: "1".into(),
        user: "sa".into(),
        password: "x".into(),
        ..Default::default()
    };
    let err: Error = connect_session_or_direct(&cfg)
        .await
        .err()
        .expect("fallback direct connect should fail against closed port");
    assert!(
        err.exit_code() == EXIT_CONN || err.to_string().to_lowercase().contains("connect"),
        "expected direct connect failure, got {err}"
    );
    assert!(
        !err.to_string().contains("configuration error: rmigd"),
        "BG-003 regression: must not stop at daemon config error"
    );
}

async fn recreate_database(database: &str) {
    let cfg = Config {
        server: std::env::var("RM_DB_SERVER").unwrap_or_else(|_| "localhost".into()),
        port: std::env::var("RM_DB_PORT").unwrap_or_else(|_| "1433".into()),
        user: std::env::var("RM_DB_USER").unwrap_or_else(|_| "sa".into()),
        password: std::env::var("RM_DB_PASSWORD")
            .unwrap_or_else(|_| "yourStrong(!)Password".into()),
        database: "master".into(),
        encrypt: false,
        trust_server_certificate: true,
        ..Default::default()
    };
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
async fn missing_daemon_socket_plan_still_succeeds_integration_happy_path() {
    if !integration_enabled() {
        return;
    }
    let tmp = tempfile::tempdir().expect("tempdir");
    let db = "sessionfallback";
    recreate_database(db).await;
    let root = tmp.path();
    std::fs::create_dir_all(root.join(format!("{db}/smoke/tables"))).expect("mkdir layout");
    std::fs::write(
        root.join(format!("{db}/smoke/tables/t1.sql")),
        "CREATE TABLE smoke.t1 (id INT NOT NULL);\n",
    )
    .expect("write table sql");

    let mut cfg = parity_cfg(root.to_str().expect("utf8 path"));
    cfg.session_socket = format!("/tmp/rmig-missing-{}.sock", std::process::id());
    let out = run_command(Command::Plan, &cfg)
        .await
        .expect("plan should succeed via direct fallback");
    assert_eq!(
        out.exit_code, 0,
        "plan must succeed when rmigd socket is missing"
    );
}

#[tokio::test]
async fn stale_rmig_session_does_not_force_exit_config_integration_regression() {
    if !integration_enabled() {
        return;
    }
    let tmp = tempfile::tempdir().expect("tempdir");
    let db = "sessionfallbackreg";
    recreate_database(db).await;
    let root = tmp.path();
    std::fs::create_dir_all(root.join(format!("{db}/smoke/tables"))).expect("mkdir layout");
    std::fs::write(
        root.join(format!("{db}/smoke/tables/t1.sql")),
        "CREATE TABLE smoke.t1 (id INT NOT NULL);\n",
    )
    .expect("write table sql");

    let mut cfg = parity_cfg(root.to_str().expect("utf8 path"));
    cfg.session_socket = "/tmp/rmig-definitely-not-running.sock".into();
    let out = run_command(Command::Plan, &cfg)
        .await
        .expect("BG-003 regression: stale RMIG_SESSION must fall back to direct connect");
    assert_ne!(
        out.exit_code, EXIT_CONFIG,
        "stale RMIG_SESSION must not yield EXIT_CONFIG when SQL is reachable"
    );
    assert_eq!(out.exit_code, 0);
}
