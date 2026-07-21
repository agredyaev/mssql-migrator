//! Rust-only e2e: cold DB → migrate (create objects) → audit metadata in `azdo_deploy_meta.history`.
//!
//! Run:
//!   RMIG_RUN_SQLSERVER_INTEGRATION=1 cargo test -p migrator-core --test apply_e2e_integration -- --nocapture --test-threads=1

#[path = "common/integration_enabled.rs"]
mod integration_enabled;

#[path = "common/workflow_config.rs"]
mod workflow_config;

#[path = "common/db_reset.rs"]
mod db_reset;

#[path = "common/state_smoke.rs"]
mod state_smoke;

#[path = "common/state_smoke_conn.rs"]
mod state_smoke_conn;

#[path = "common/engine_smoke.rs"]
mod engine_smoke;

use migrator_core::domain::Action;

const MIN_SMOKE_OBJECTS: i32 = 6;

/// Empty DB: `migrate` creates schema + smoke objects and writes object rows to audit history.
#[tokio::test(flavor = "current_thread")]
async fn apply_e2e_cold_migrate_creates_objects_and_audit() {
    if !integration_enabled::enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }

    let cfg = workflow_config::workflow_config();
    let mut cold = cfg.clone();
    cold.skip_git = true;

    db_reset::reset_test_database(&cold)
        .await
        .expect("reset db");

    let out = engine_smoke::baseline_migrate(&cold)
        .await
        .expect("baseline migrate");
    assert_eq!(out.exit_code, 0, "baseline migrate must succeed");

    let mut conn = state_smoke_conn::open_conn(&cold).await.expect("connect");
    state_smoke::assert_smoke_objects_materialized(&mut conn)
        .await
        .expect("smoke objects in sys catalog");

    let audit = state_smoke_conn::count_audit_rows(&mut conn, "object", "applied")
        .await
        .expect("audit count");
    assert!(
        audit >= MIN_SMOKE_OBJECTS,
        "expected >= {MIN_SMOKE_OBJECTS} object audit rows, got {audit}"
    );

    let (plan, _) = engine_smoke::plan(&cold).await.expect("warm plan");
    let creates = plan
        .objects
        .iter()
        .filter(|o| o.planned_action == Action::CreateObject)
        .count();
    assert_eq!(
        creates, 0,
        "warm plan must not recreate objects already in DB"
    );
    let unchanged = plan
        .objects
        .iter()
        .filter(|o| {
            matches!(
                o.planned_action,
                Action::SkipUnchanged | Action::AdoptExisting
            )
        })
        .count();
    assert!(
        unchanged >= MIN_SMOKE_OBJECTS as usize,
        "warm plan should skip/adopt smoke objects, got {unchanged}"
    );

    eprintln!("apply_e2e_cold OK: audit_object_rows={audit} warm_skip_or_adopt={unchanged}");
}
