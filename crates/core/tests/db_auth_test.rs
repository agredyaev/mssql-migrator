use migrator_core::config::validate_config;
use migrator_core::driver::{connect, mssql};
use migrator_core::engine::{run_command, Command};
use migrator_core::Config;

fn integration_enabled() -> bool {
    std::env::var("RMIG_RUN_SQLSERVER_INTEGRATION")
        .map(|v| v == "1" || v.eq_ignore_ascii_case("true"))
        .unwrap_or(false)
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

    let sql_root: String = root.to_str().expect("utf8").into();
    let mut cfg = Config {
        server: std::env::var("RM_DB_SERVER").unwrap_or_else(|_| "localhost".into()),
        port: std::env::var("RM_DB_PORT").unwrap_or_else(|_| "1433".into()),
        user: std::env::var("RM_DB_USER").unwrap_or_else(|_| "sa".into()),
        password: std::env::var("RM_DB_PASSWORD")
            .unwrap_or_else(|_| "yourStrong(!)Password".into()),
        sql_root: sql_root.clone(),
        sql_base: sql_root,
        skip_git: true,
        encrypt: false,
        trust_server_certificate: true,
        ..Default::default()
    };
    validate_config(&mut cfg).expect("valid cfg");
    let out = run_command(Command::Plan, &cfg)
        .await
        .expect("sql auth plan");
    assert_eq!(out.exit_code, 0);
}
