use migrator_core::config::ensure_catalog_databases_exist;
use migrator_core::driver::{connect, mssql};
use migrator_core::Config;

fn integration_enabled() -> bool {
    std::env::var("RMIG_RUN_SQLSERVER_INTEGRATION")
        .map(|v| v == "1" || v.eq_ignore_ascii_case("true"))
        .unwrap_or(false)
}

fn sa_cfg(database: &str) -> Config {
    let mut cfg = Config::default();
    cfg.server = std::env::var("RM_DB_SERVER").unwrap_or_else(|_| "localhost".into());
    cfg.port = std::env::var("RM_DB_PORT").unwrap_or_else(|_| "1433".into());
    cfg.user = std::env::var("RM_DB_USER").unwrap_or_else(|_| "sa".into());
    cfg.password =
        std::env::var("RM_DB_PASSWORD").unwrap_or_else(|_| "yourStrong(!)Password".into());
    cfg.database = database.into();
    cfg.encrypt = false;
    cfg.trust_server_certificate = true;
    cfg
}

fn contained_cfg(database: &str) -> Config {
    let mut cfg = Config::default();
    cfg.server = std::env::var("RM_DB_SERVER").unwrap_or_else(|_| "localhost".into());
    cfg.port = std::env::var("RM_DB_PORT").unwrap_or_else(|_| "1433".into());
    cfg.user = "rmig_plan_contained".into();
    cfg.password = "ContainedPass123".into();
    cfg.database = database.into();
    cfg.encrypt = false;
    cfg.trust_server_certificate = true;
    cfg
}

async fn recreate_contained_database(database: &str, contained_user: &str, password: &str) {
    let mut master = connect(&sa_cfg("master"))
        .await
        .expect("connect master as sa");
    mssql::exec(
        &mut master.client,
        "EXEC sp_configure 'show advanced options', 1; RECONFIGURE; \
         EXEC sp_configure 'contained database authentication', 1; RECONFIGURE;",
    )
    .await
    .expect("enable contained database authentication");
    let drop_sql = format!(
        "IF DB_ID(N'{0}') IS NOT NULL BEGIN ALTER DATABASE [{0}] SET SINGLE_USER WITH ROLLBACK IMMEDIATE; DROP DATABASE [{0}]; END",
        database
    );
    mssql::exec(&mut master.client, &drop_sql)
        .await
        .expect("drop contained db if exists");
    let create_sql = format!("CREATE DATABASE [{database}]");
    mssql::exec(&mut master.client, &create_sql)
        .await
        .expect("create contained db");

    let mut db = connect(&sa_cfg(database))
        .await
        .expect("connect contained db as sa");
    let setup_sql = format!(
        "ALTER DATABASE [{db}] SET CONTAINMENT = PARTIAL;\
         IF EXISTS (SELECT 1 FROM sys.database_principals WHERE name = '{user}') DROP USER [{user}];\
         CREATE USER [{user}] WITH PASSWORD = '{password}';\
         ALTER ROLE [db_owner] ADD MEMBER [{user}];",
        db = database,
        user = contained_user,
        password = password
    );
    mssql::exec(&mut db.client, &setup_sql)
        .await
        .expect("create contained user");
}

#[tokio::test]
async fn plan_existing_database_with_sa_happy_path() {
    if !integration_enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }

    recreate_contained_database("containedplan", "rmig_plan_contained", "ContainedPass123").await;
    let cfg = sa_cfg("containedplan");
    ensure_catalog_databases_exist(&cfg, &["containedplan".to_string()])
        .await
        .expect("sa should ensure existing db");
}

#[tokio::test]
async fn plan_missing_database_with_contained_user_fails_negative_path() {
    if !integration_enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }

    let cfg = contained_cfg("missingcontained");
    let err = match ensure_catalog_databases_exist(&cfg, &["missingcontained".to_string()]).await {
        Ok(_) => panic!("contained user should not create missing db"),
        Err(err) => err,
    };
    let msg = err.to_string();
    assert!(
        msg.contains("Login failed for user 'rmig_plan_contained'") || msg.contains("master"),
        "unexpected error: {msg}"
    );
}

#[tokio::test]
async fn ensure_existing_and_missing_databases_mixed_edge_case() {
    if !integration_enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }

    recreate_contained_database("containedplan", "rmig_plan_contained", "ContainedPass123").await;

    let cfg = contained_cfg("");
    let err = match ensure_catalog_databases_exist(
        &cfg,
        &["containedplan".to_string(), "missingcontained".to_string()],
    )
    .await
    {
        Ok(_) => panic!("mixed dbs should fail on inaccessible missing db"),
        Err(err) => err,
    };
    let msg = err.to_string();
    assert!(
        msg.contains("Login failed for user 'rmig_plan_contained'") || msg.contains("master"),
        "unexpected error: {msg}"
    );
}

#[tokio::test]
async fn plan_existing_database_with_contained_user_regression() {
    if !integration_enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }

    recreate_contained_database("containedplan", "rmig_plan_contained", "ContainedPass123").await;
    let cfg = contained_cfg("containedplan");
    ensure_catalog_databases_exist(&cfg, &["containedplan".to_string()])
        .await
        .expect("contained user should skip master preflight on existing db");
}
