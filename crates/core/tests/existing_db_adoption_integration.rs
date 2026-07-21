//! Existing-database safety e2e: objects that live in the database but are NOT
//! represented in the repository tree must be preserved across `migrate`, and a
//! dry-run `plan` must neither reference nor touch them.
//!
//! These verify, against a real SQL Server, the "safe by default" contract from
//! `docs/repository-contract.md` and `docs/migration-flow.md`.
//!
//! Run (serially — every test resets the shared database):
//!   RMIG_RUN_SQLSERVER_INTEGRATION=1 cargo test -p migrator-core \
//!     --test existing_db_adoption_integration -- --nocapture --test-threads=1

#[path = "common/integration_enabled.rs"]
mod integration_enabled;

#[path = "common/workflow_config.rs"]
mod workflow_config;

#[path = "common/db_reset.rs"]
mod db_reset;

#[path = "common/engine_smoke.rs"]
mod engine_smoke;

#[path = "common/state_smoke.rs"]
mod state_smoke;

#[path = "common/state_smoke_conn.rs"]
mod state_smoke_conn;

use migrator_core::config::Config;
use migrator_core::driver::TimingConn;

/// Fresh database + a managed-objects config (skip_git for deterministic plans).
async fn fresh_cold_db() -> Config {
    let mut cold = workflow_config::workflow_config().clone();
    cold.skip_git = true;
    db_reset::reset_test_database(&cold)
        .await
        .expect("reset db");
    cold
}

/// Create a schema + table that exist only in the database (never in the repo tree).
async fn create_unmanaged_table(conn: &mut TimingConn, schema: &str, table: &str) {
    conn.exec(&format!(
        "IF SCHEMA_ID(N'{schema}') IS NULL EXEC('CREATE SCHEMA [{schema}]')"
    ))
    .await
    .expect("create unmanaged schema");
    conn.exec(&format!(
        "CREATE TABLE [{schema}].[{table}] (id INT NOT NULL)"
    ))
    .await
    .expect("create unmanaged table");
}

#[tokio::test(flavor = "current_thread")]
async fn existing_db_object_absent_from_repo_survives_migrate() {
    if !integration_enabled::enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }
    let cold = fresh_cold_db().await;
    let mut conn = state_smoke_conn::open_conn(&cold).await.expect("connect");

    create_unmanaged_table(&mut conn, "unmanaged_ns", "keep_me").await;
    assert!(
        state_smoke::user_table_exists(&mut conn, "unmanaged_ns", "keep_me")
            .await
            .unwrap()
    );

    let out = engine_smoke::baseline_migrate(&cold)
        .await
        .expect("migrate");
    assert_eq!(out.exit_code, 0, "migrate of managed objects must succeed");

    // Managed objects were created; the unmanaged object is untouched.
    state_smoke::assert_smoke_objects_materialized(&mut conn)
        .await
        .expect("managed objects created");
    assert!(
        state_smoke::user_table_exists(&mut conn, "unmanaged_ns", "keep_me")
            .await
            .unwrap(),
        "unmanaged DB object must survive migrate"
    );
}

#[tokio::test(flavor = "current_thread")]
async fn partial_repo_migrate_does_not_drop_unrelated_objects() {
    if !integration_enabled::enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }
    let cold = fresh_cold_db().await;
    let mut conn = state_smoke_conn::open_conn(&cold).await.expect("connect");

    // Several unrelated objects across kinds, none represented in the repo.
    create_unmanaged_table(&mut conn, "legacy", "orders").await;
    conn.exec("CREATE VIEW [legacy].[orders_v] AS SELECT id FROM [legacy].[orders]")
        .await
        .expect("create unmanaged view");

    let out = engine_smoke::baseline_migrate(&cold)
        .await
        .expect("migrate");
    assert_eq!(out.exit_code, 0);

    state_smoke::assert_smoke_objects_materialized(&mut conn)
        .await
        .expect("managed objects created");
    assert!(
        state_smoke::user_table_exists(&mut conn, "legacy", "orders")
            .await
            .unwrap()
    );
    assert!(state_smoke::view_exists(&mut conn, "legacy", "orders_v")
        .await
        .unwrap());
}

#[tokio::test(flavor = "current_thread")]
async fn dry_run_plan_ignores_unmanaged_objects_and_reports_nothing_destructive() {
    if !integration_enabled::enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }
    let cold = fresh_cold_db().await;
    let mut conn = state_smoke_conn::open_conn(&cold).await.expect("connect");
    create_unmanaged_table(&mut conn, "unmanaged_ns", "keep_me").await;

    engine_smoke::baseline_migrate(&cold)
        .await
        .expect("migrate");

    let audit_before = state_smoke_conn::count_audit_rows(&mut conn, "object", "applied")
        .await
        .expect("audit count before");
    let (plan, _) = engine_smoke::plan(&cold).await.expect("dry-run plan");
    assert!(!plan.blocked, "clean warm plan must not be blocked");
    // The unmanaged object is never referenced by the plan.
    assert!(
        plan.objects
            .iter()
            .all(|o| !o.normalized_key.as_ref().starts_with("unmanaged_ns/")),
        "dry-run plan must not reference unmanaged objects"
    );
    // Plan is read-only: the unmanaged object still exists afterwards.
    assert!(
        state_smoke::user_table_exists(&mut conn, "unmanaged_ns", "keep_me")
            .await
            .unwrap(),
        "dry-run plan must not drop the unmanaged object"
    );
    // Plan wrote nothing: audit history is unchanged.
    let audit_after = state_smoke_conn::count_audit_rows(&mut conn, "object", "applied")
        .await
        .expect("audit count after");
    assert_eq!(
        audit_before, audit_after,
        "dry-run plan must not write audit rows"
    );
}
