use migrator_core::audit::db_fingerprint;
use migrator_core::config::{discover_catalog_databases, validate_config};
use migrator_core::driver::{connect, mssql};
use migrator_core::engine::{run_command, Command};
use migrator_core::Config;

fn integration_enabled() -> bool {
    std::env::var("RMIG_RUN_SQLSERVER_INTEGRATION")
        .map(|v| v == "1" || v.eq_ignore_ascii_case("true"))
        .unwrap_or(false)
}

fn sa_cfg(database: &str, sql_root: &str) -> Config {
    let mut cfg = Config::default();
    cfg.server = std::env::var("RM_DB_SERVER").unwrap_or_else(|_| "localhost".into());
    cfg.port = std::env::var("RM_DB_PORT").unwrap_or_else(|_| "1433".into());
    cfg.user = std::env::var("RM_DB_USER").unwrap_or_else(|_| "sa".into());
    cfg.password =
        std::env::var("RM_DB_PASSWORD").unwrap_or_else(|_| "yourStrong(!)Password".into());
    cfg.database = database.into();
    cfg.sql_root = sql_root.into();
    cfg.sql_base = sql_root.into();
    cfg.set_skip_git(true);
    cfg.set_encrypt(false);
    cfg.set_trust_server_certificate(true);
    cfg
}

fn write_sql_layout(root: &std::path::Path, db: &str) {
    std::fs::create_dir_all(root.join(format!("{db}/smoke/tables"))).expect("mkdir layout");
    std::fs::write(
        root.join(format!("{db}/smoke/tables/t1.sql")),
        "CREATE TABLE smoke.t1 (id INT NOT NULL);\n",
    )
    .expect("write table sql");
}

async fn drop_database_if_exists(database: &str) {
    let mut master = connect(&sa_cfg("master", ""))
        .await
        .expect("connect master");
    let sql = format!(
        "IF DB_ID(N'{0}') IS NOT NULL BEGIN ALTER DATABASE [{0}] SET SINGLE_USER WITH ROLLBACK IMMEDIATE; DROP DATABASE [{0}]; END",
        database.replace('\'', "''")
    );
    mssql::exec(&mut master.client, &sql)
        .await
        .expect("drop database if exists");
}

async fn database_exists(database: &str) -> bool {
    let mut master = connect(&sa_cfg("master", ""))
        .await
        .expect("connect master");
    let escaped = database.replace('\'', "''");
    let sql = format!(
        "SELECT CASE WHEN DB_ID(N'{escaped}') IS NULL THEN CAST(0 AS bit) ELSE CAST(1 AS bit) END"
    );
    let rows = mssql::query_tiberius(&mut master.client, &sql, &[])
        .await
        .expect("probe db_id");
    rows.first()
        .and_then(|r| r.get::<bool, _>(0))
        .unwrap_or(false)
}

#[tokio::test]
async fn plan_existing_database_happy_path() {
    if !integration_enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }

    let base = tempfile::tempdir().expect("tempdir");
    write_sql_layout(base.path(), "sideeffect_existing");

    drop_database_if_exists("sideeffect_existing").await;
    let mut cfg = sa_cfg("sideeffect_existing", &base.path().to_string_lossy());
    validate_config(&mut cfg).expect("valid config");
    migrator_core::config::ensure_catalog_databases_exist(
        &cfg,
        &["sideeffect_existing".to_string()],
    )
    .await
    .expect("test setup creates db");

    let out = run_command(Command::Plan, &cfg)
        .await
        .expect("plan on existing db");
    let plan = out.plan.expect("plan output");
    assert_eq!(plan.summary.object_count, 1);
}

#[tokio::test]
async fn plan_missing_database_fails_without_create_negative_path() {
    if !integration_enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }

    let base = tempfile::tempdir().expect("tempdir");
    write_sql_layout(base.path(), "sideeffect_missing_neg");

    drop_database_if_exists("sideeffect_missing_neg").await;
    assert!(
        !database_exists("sideeffect_missing_neg").await,
        "precondition: database must not exist"
    );

    let mut cfg = sa_cfg("sideeffect_missing_neg", &base.path().to_string_lossy());
    validate_config(&mut cfg).expect("valid config");

    let err = match run_command(Command::Plan, &cfg).await {
        Ok(_) => panic!("plan should fail when target database is missing"),
        Err(err) => err,
    };
    assert!(
        !database_exists("sideeffect_missing_neg").await,
        "plan must not create database, but sideeffect_missing_neg now exists: {err}"
    );
}

#[tokio::test]
async fn validate_missing_database_edge_case() {
    if !integration_enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }

    let base = tempfile::tempdir().expect("tempdir");
    write_sql_layout(base.path(), "sideeffect_validate_missing");

    drop_database_if_exists("sideeffect_validate_missing").await;
    assert!(
        !database_exists("sideeffect_validate_missing").await,
        "precondition: database must not exist"
    );

    let mut cfg = sa_cfg(
        "sideeffect_validate_missing",
        &base.path().to_string_lossy(),
    );
    validate_config(&mut cfg).expect("valid config");
    assert!(
        connect(&cfg).await.is_err(),
        "direct connect to missing database should fail"
    );

    let err = match run_command(Command::Validate, &cfg).await {
        Ok(_) => panic!("validate should fail when target database is missing"),
        Err(err) => err,
    };
    assert!(
        !database_exists("sideeffect_validate_missing").await,
        "validate must not create missing db: {err}"
    );
}

#[tokio::test]
async fn plan_multi_db_layout_only_targets_catalog_directories_edge_case() {
    if !integration_enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }

    let base = tempfile::tempdir().expect("tempdir");
    write_sql_layout(base.path(), "sideeffect_multi_present");
    write_sql_layout(base.path(), "sideeffect_multi_absent");

    let dbs = discover_catalog_databases(&base.path().to_string_lossy()).expect("discover");
    assert_eq!(dbs.len(), 2);

    drop_database_if_exists("sideeffect_multi_present").await;
    drop_database_if_exists("sideeffect_multi_absent").await;
    migrator_core::config::ensure_catalog_databases_exist(
        &sa_cfg("sideeffect_multi_present", &base.path().to_string_lossy()),
        &["sideeffect_multi_present".to_string()],
    )
    .await
    .expect("setup existing db only");

    let mut cfg = sa_cfg("", &base.path().to_string_lossy());
    validate_config(&mut cfg).expect("valid config");
    assert!(
        cfg.database.is_empty(),
        "multi-db catalog should stay unresolved"
    );

    let _ = run_command(Command::Plan, &cfg)
        .await
        .expect("plan may succeed for reachable catalog databases only");
    assert!(database_exists("sideeffect_multi_present").await);
    assert!(
        !database_exists("sideeffect_multi_absent").await,
        "plan must not auto-create the missing catalog database"
    );
}

#[tokio::test]
async fn plan_missing_database_does_not_create_regression() {
    if !integration_enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }

    let base = tempfile::tempdir().expect("tempdir");
    write_sql_layout(base.path(), "sideeffect_regression");

    drop_database_if_exists("sideeffect_regression").await;
    migrator_core::audit::invalidate_audit_cache_all(&db_fingerprint(
        &std::env::var("RM_DB_SERVER").unwrap_or_else(|_| "localhost".into()),
        "sideeffect_regression",
    ));

    let mut cfg = sa_cfg("sideeffect_regression", &base.path().to_string_lossy());
    validate_config(&mut cfg).expect("valid config");

    // Before BG-006 fix, sa-backed plan auto-created the catalog database here.
    let err = match run_command(Command::Plan, &cfg).await {
        Ok(out) => {
            let objects = out
                .plan
                .as_ref()
                .map(|p| p.summary.object_count)
                .unwrap_or(0);
            panic!("regression: plan created db and returned {objects} objects");
        }
        Err(err) => err,
    };

    assert!(
        !database_exists("sideeffect_regression").await,
        "regression: plan still creates database on sa login: {err}"
    );
}
