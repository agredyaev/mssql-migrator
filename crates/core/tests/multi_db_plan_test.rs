use migrator_core::audit::{self, invalidate_audit_cache_all};
use migrator_core::cache::l1::L1Cache;
use migrator_core::config::validate_config;
use migrator_core::db::invalidate_inspect_cache;
use migrator_core::driver::{connect, mssql};
use migrator_core::engine::{run_command, Command};
use migrator_core::export::MigrationPlan;
use migrator_core::Config;
use std::collections::BTreeSet;
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

fn parity_cfg(root: &Path) -> Config {
    let mut cfg = connect_cfg("");
    cfg.sql_root = root.to_string_lossy().into_owned();
    cfg.sql_base = cfg.sql_root.clone();
    cfg.set_skip_git(true);
    validate_config(&mut cfg).expect("valid config");
    cfg
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
    let l1 = L1Cache::new(".rmig/cache");
    let _ = l1.invalidate_all(&db_fp);
}

async fn prepare_catalog_databases(databases: &[&str]) {
    for database in databases {
        recreate_empty_database(database).await;
    }
}

fn write_multi_db_layout(root: &Path) {
    std::fs::create_dir_all(root.join("dactests/smoke/tables")).expect("mkdir dactests");
    std::fs::create_dir_all(root.join("warehouse/reporting/tables")).expect("mkdir warehouse");
    std::fs::write(
        root.join("dactests/smoke/tables/smoke_table.sql"),
        "CREATE TABLE smoke.smoke_table(id INT NOT NULL);\n",
    )
    .expect("write dactests sql");
    std::fs::write(
        root.join("warehouse/reporting/tables/fact_table.sql"),
        "CREATE TABLE reporting.fact_table(id INT NOT NULL);\n",
    )
    .expect("write warehouse sql");
}

fn write_single_db_layout(root: &Path, db: &str) {
    std::fs::create_dir_all(root.join(format!("{db}/smoke/tables"))).expect("mkdir db");
    std::fs::write(
        root.join(format!("{db}/smoke/tables/only_table.sql")),
        "CREATE TABLE smoke.only_table(id INT NOT NULL);\n",
    )
    .expect("write table sql");
}

fn database_names(plan: &MigrationPlan) -> BTreeSet<String> {
    plan.objects
        .iter()
        .map(|o| o.database_name.as_ref().to_string())
        .collect()
}

#[tokio::test]
async fn multi_db_plan_returns_both_catalogs_happy_path() {
    if !integration_enabled() {
        return;
    }
    let base = tempfile::tempdir().expect("tempdir");
    write_multi_db_layout(base.path());
    prepare_catalog_databases(&["dactests", "warehouse"]).await;
    let cfg = parity_cfg(base.path());

    let out = run_command(Command::Plan, &cfg)
        .await
        .expect("multi db plan");
    let plan = out.plan.expect("plan output");
    assert_eq!(plan.summary.object_count, 2);
    assert_eq!(plan.objects.len(), 2);
    assert_eq!(
        database_names(&plan),
        BTreeSet::from(["dactests".to_string(), "warehouse".to_string()])
    );
}

#[tokio::test]
async fn single_database_layout_returns_one_catalog_negative_path() {
    if !integration_enabled() {
        return;
    }
    let base = tempfile::tempdir().expect("tempdir");
    write_single_db_layout(base.path(), "dactests");
    prepare_catalog_databases(&["dactests"]).await;
    let cfg = parity_cfg(base.path());

    let out = run_command(Command::Plan, &cfg)
        .await
        .expect("single db plan");
    let plan = out.plan.expect("plan output");
    assert_eq!(plan.summary.object_count, 1);
    assert_eq!(plan.objects.len(), 1);
    assert_eq!(
        database_names(&plan),
        BTreeSet::from(["dactests".to_string()])
    );
}

#[tokio::test]
async fn multi_db_plan_summary_matches_merged_objects_edge_case() {
    if !integration_enabled() {
        return;
    }
    let base = tempfile::tempdir().expect("tempdir");
    write_multi_db_layout(base.path());
    prepare_catalog_databases(&["dactests", "warehouse"]).await;
    let cfg = parity_cfg(base.path());

    let out = run_command(Command::Plan, &cfg)
        .await
        .expect("multi db plan");
    let plan = out.plan.expect("plan output");
    assert_eq!(
        plan.summary.object_count as usize,
        plan.objects.len(),
        "summary must reflect merged objects"
    );
    assert_eq!(plan.rows.len(), plan.objects.len());
}

#[tokio::test]
async fn multi_db_plan_preserves_first_database_objects_regression() {
    if !integration_enabled() {
        return;
    }
    let base = tempfile::tempdir().expect("tempdir");
    write_multi_db_layout(base.path());
    prepare_catalog_databases(&["dactests", "warehouse"]).await;
    let cfg = parity_cfg(base.path());

    let out = run_command(Command::Plan, &cfg)
        .await
        .expect("BG-015 regression plan");
    let plan = out.plan.expect("plan output");
    let keys: BTreeSet<String> = plan
        .objects
        .iter()
        .map(|o| o.normalized_key.as_ref().to_string())
        .collect();
    assert!(
        keys.iter().any(|k| k.contains("smoke_table")),
        "BG-015 regression: first database objects must survive merge, got {keys:?}"
    );
    assert!(
        keys.iter().any(|k| k.contains("fact_table")),
        "BG-015 regression: last database objects must be present, got {keys:?}"
    );
}
