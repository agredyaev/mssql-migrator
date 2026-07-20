use migrator_core::audit::{db_fingerprint, invalidate_audit_cache_all};
use migrator_core::config::{ensure_catalog_databases_exist, validate_config};
use migrator_core::driver::{connect, mssql};
use migrator_core::engine::{run_command, Command};
use migrator_core::Config;

#[path = "common/pipeline.rs"]
mod pipeline;

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
    cfg.set_encrypt(false);
    cfg.set_trust_server_certificate(true);
    cfg
}

fn contained_cfg(database: &str, sql_root: &str) -> Config {
    let mut cfg = sa_cfg(database);
    cfg.user = "rmig_plan_contained".into();
    cfg.password = "ContainedPass123".into();
    cfg.sql_root = sql_root.into();
    cfg.sql_base = sql_root.into();
    cfg.set_skip_git(true);
    cfg
}

async fn recreate_collation_database(database: &str, contained_user: &str, password: &str) {
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
        .expect("drop collation db if exists");
    let create_sql =
        format!("CREATE DATABASE [{database}] COLLATE Latin1_General_100_CI_AS_KS_WS_SC");
    mssql::exec(&mut master.client, &create_sql)
        .await
        .expect("create collation db");

    let mut db = connect(&sa_cfg(database))
        .await
        .expect("connect collation db as sa");
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

    let cfg = sa_cfg(database);
    invalidate_audit_cache_all(&db_fingerprint(
        &cfg.server,
        &cfg.port,
        &cfg.user,
        &cfg.database,
    ));
}

fn write_sql_layout(root: &std::path::Path) {
    std::fs::create_dir_all(root.join("collationplan/smoke/tables")).expect("mkdir layout");
    std::fs::write(
        root.join("collationplan/smoke/tables/t1.sql"),
        "CREATE TABLE smoke.t1 (id INT NOT NULL);\n",
    )
    .expect("write table sql");
}

#[tokio::test]
async fn plan_latin1_contained_database_happy_path() {
    if !integration_enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }

    let base = tempfile::tempdir().expect("tempdir");
    write_sql_layout(base.path());

    recreate_collation_database("collationplan", "rmig_plan_contained", "ContainedPass123").await;

    let mut cfg = contained_cfg("collationplan", &base.path().to_string_lossy());
    validate_config(&mut cfg).expect("valid config");
    ensure_catalog_databases_exist(&cfg, &["collationplan".to_string()])
        .await
        .expect("ensure existing db");

    let out = run_command(Command::Plan, &cfg)
        .await
        .expect("plan should succeed on Latin1_General contained db");
    let plan = out.plan.expect("plan output");
    assert_eq!(
        plan.summary.object_count, 1,
        "unexpected summary: {:?}",
        plan.summary
    );
}

#[tokio::test]
async fn plan_latin1_contained_database_negative_path() {
    if !integration_enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }

    let base = tempfile::tempdir().expect("tempdir");
    write_sql_layout(base.path());

    recreate_collation_database("collationplan", "rmig_plan_contained", "ContainedPass123").await;

    let mut cfg = contained_cfg("missingcollationplan", &base.path().to_string_lossy());
    validate_config(&mut cfg).expect("valid config");
    let err = match run_command(Command::Plan, &cfg).await {
        Ok(_) => panic!("plan should fail when target database is missing"),
        Err(err) => err,
    };
    let msg = err.to_string();
    assert!(
        msg.contains("Login failed for user 'rmig_plan_contained'") || msg.contains("master"),
        "unexpected error: {msg}"
    );
}

#[tokio::test]
async fn plan_latin1_contained_database_empty_layout_edge_case() {
    if !integration_enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }

    let base = tempfile::tempdir().expect("tempdir");
    std::fs::create_dir_all(base.path().join("collationplan/smoke/tables")).expect("mkdir empty");

    recreate_collation_database("collationplan", "rmig_plan_contained", "ContainedPass123").await;

    let mut cfg = contained_cfg("collationplan", &base.path().to_string_lossy());
    validate_config(&mut cfg).expect("valid config");

    let out = run_command(Command::Plan, &cfg)
        .await
        .expect("empty layout plan should not hit collation conflict");
    let plan = out.plan.expect("plan output");
    assert_eq!(plan.summary.object_count, 0);
}

#[tokio::test]
async fn plan_latin1_contained_user_regression() {
    if !integration_enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }

    let base = tempfile::tempdir().expect("tempdir");
    write_sql_layout(base.path());

    recreate_collation_database("collationplan", "rmig_plan_contained", "ContainedPass123").await;

    let mut cfg = contained_cfg("collationplan", &base.path().to_string_lossy());
    validate_config(&mut cfg).expect("valid config");

    let (plan, _, _) = pipeline::run_plan_pipeline(&cfg)
        .await
        .expect("pipeline plan should not fail with collation conflict");
    assert_eq!(plan.summary.object_count, 1);
    assert!(
        !plan.objects.is_empty(),
        "expected materialized object for smoke/t1"
    );
}
