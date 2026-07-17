use migrator_core::audit::{self, invalidate_audit_cache_all};
use migrator_core::cache::l1::L1Cache;
use migrator_core::config::validate_config;
use migrator_core::db::invalidate_inspect_cache;
use migrator_core::driver::{connect, mssql};
use migrator_core::engine::{run_command, Command};
use migrator_core::Config;
use std::path::Path;

fn integration_enabled() -> bool {
    std::env::var("RMIG_RUN_SQLSERVER_INTEGRATION")
        .map(|v| v == "1" || v.eq_ignore_ascii_case("true"))
        .unwrap_or(false)
}

fn connect_cfg(database: &str) -> Config {
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

fn plan_cfg(database: &str, sql_root: &str) -> Config {
    let mut cfg = connect_cfg(database);
    cfg.sql_root = sql_root.into();
    cfg.sql_base = sql_root.into();
    cfg.set_skip_git(true);
    validate_config(&mut cfg).expect("valid cfg");
    cfg
}

fn write_layout(root: &Path, db: &str) {
    std::fs::create_dir_all(root.join(format!("{db}/smoke/tables"))).expect("mkdir layout");
    std::fs::write(
        root.join(format!("{db}/smoke/tables/t1.sql")),
        "CREATE TABLE smoke.t1 (id INT NOT NULL);\n",
    )
    .expect("write table sql");
}

async fn recreate_empty_database(database: &str) {
    let mut master = connect(&connect_cfg("master"))
        .await
        .expect("connect master");
    let escaped = database.replace('\'', "''");
    let sql = format!(
        "IF DB_ID(N'{escaped}') IS NOT NULL BEGIN ALTER DATABASE [{database}] SET SINGLE_USER WITH ROLLBACK IMMEDIATE; DROP DATABASE [{database}]; END; CREATE DATABASE [{database}];"
    );
    mssql::exec(&mut master.client, &sql)
        .await
        .expect("recreate database");
    let db_fp = audit::db_fingerprint(
        &std::env::var("RM_DB_SERVER").unwrap_or_else(|_| "localhost".into()),
        &std::env::var("RM_DB_PORT").unwrap_or_else(|_| "1433".into()),
        &std::env::var("RM_DB_USER").unwrap_or_else(|_| "sa".into()),
        database,
    );
    invalidate_audit_cache_all(&db_fp);
    invalidate_inspect_cache(&db_fp);
    migrator_core::db::warm_snapshot::clear();
    let l1_dir = std::env::temp_dir().join(format!("rmig-deferred-bootstrap-{database}"));
    let _ = std::fs::remove_dir_all(&l1_dir);
    let l1 = L1Cache::new(&l1_dir.to_string_lossy());
    let _ = l1.invalidate_all(&db_fp);
}

#[tokio::test]
async fn plan_fresh_database_without_audit_tables_happy_path() {
    if !integration_enabled() {
        return;
    }
    let base = tempfile::tempdir().expect("tempdir");
    let db = "deferredbootstrap";
    write_layout(base.path(), db);
    recreate_empty_database(db).await;
    let cfg = plan_cfg(db, base.path().to_str().unwrap());

    let out = run_command(Command::Plan, &cfg)
        .await
        .expect("plan on fresh db without pre-created audit tables");
    let plan = out.plan.expect("plan output");
    assert_eq!(plan.summary.object_count, 1);
}

#[tokio::test]
async fn plan_missing_database_fails_without_bootstrap_negative_path() {
    if !integration_enabled() {
        return;
    }
    let base = tempfile::tempdir().expect("tempdir");
    let db = "missingdeferred";
    write_layout(base.path(), db);
    let cfg = plan_cfg(db, base.path().to_str().unwrap());
    let err = run_command(Command::Plan, &cfg)
        .await
        .err()
        .expect("plan must fail when catalog database does not exist");
    let msg = err.to_string();
    assert!(
        !msg.contains("azdo_deploy_meta.history"),
        "negative path should fail before history probe, got {msg}"
    );
}

#[tokio::test]
async fn second_plan_on_bootstrapped_database_edge_case() {
    if !integration_enabled() {
        return;
    }
    let base = tempfile::tempdir().expect("tempdir");
    let db = "deferredbootstrap2";
    write_layout(base.path(), db);
    recreate_empty_database(db).await;
    let cfg = plan_cfg(db, base.path().to_str().unwrap());

    run_command(Command::Plan, &cfg)
        .await
        .expect("first plan bootstraps audit tables");
    let out = run_command(Command::Plan, &cfg)
        .await
        .expect("second plan on bootstrapped db");
    assert_eq!(out.plan.expect("plan").summary.object_count, 1);
}

#[tokio::test]
async fn plan_fresh_database_no_history_table_error_regression() {
    if !integration_enabled() {
        return;
    }
    let base = tempfile::tempdir().expect("tempdir");
    let db = "deferredbootstrap3";
    write_layout(base.path(), db);
    recreate_empty_database(db).await;
    let cfg = plan_cfg(db, base.path().to_str().unwrap());

    let err_msg = match run_command(Command::Plan, &cfg).await {
        Ok(out) => {
            let count = out.plan.expect("plan").summary.object_count;
            assert_eq!(count, 1);
            return;
        }
        Err(err) => err.to_string(),
    };
    assert!(
        !err_msg.contains("azdo_deploy_meta.history"),
        "BG-016 regression: deferred bootstrap must not query history before bootstrap: {err_msg}"
    );
}
