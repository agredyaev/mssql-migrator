//! Adopt e2e: object pre-created in the database out-of-band, identical DDL
//! present in the repo tree → `plan` decides `AdoptExisting`, `migrate` records
//! it in audit metadata (`event = 'adopted'`) without executing DDL, and a warm
//! re-plan resolves it to `SkipUnchanged`.
//!
//! Run:
//!   RMIG_RUN_SQLSERVER_INTEGRATION=1 cargo test -p migrator-core --test adopt_e2e_integration -- --nocapture --test-threads=1

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
use migrator_core::export::MigrationPlan;

const SMOKE_TABLE_FIXTURE: &str = "dactests/smoke/tables/smoke_table.sql";

/// Pre-existing DB object with identical repo DDL is adopted, not re-created.
#[tokio::test(flavor = "current_thread")]
async fn adopt_e2e_preexisting_identical_object_is_adopted_without_ddl() {
    if !integration_enabled::enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }

    let mut cfg = workflow_config::workflow_config().clone();
    cfg.set_skip_git(true);

    db_reset::reset_test_database(&cfg).await.expect("reset db");

    // Create the object out-of-band with the exact fixture DDL, so the repo
    // script and the live object are byte-identical by construction.
    let ddl =
        std::fs::read_to_string(std::path::Path::new(&cfg.sql_root).join(SMOKE_TABLE_FIXTURE))
            .expect("read smoke_table fixture");
    let mut conn = state_smoke_conn::open_conn(&cfg).await.expect("connect");
    conn.exec("IF SCHEMA_ID(N'smoke') IS NULL EXEC('CREATE SCHEMA [smoke]')")
        .await
        .expect("create smoke schema");
    conn.exec(&ddl).await.expect("pre-create smoke_table");

    let (plan, _) = engine_smoke::plan(&cfg).await.expect("cold plan");
    assert_eq!(
        smoke_table_action(&plan),
        Action::AdoptExisting,
        "pre-existing identical object must be adopted"
    );

    // Exit 0 also proves no DDL ran for the table: a replayed CREATE TABLE
    // would fail with error 2714 and abort the migrate.
    let out = engine_smoke::baseline_migrate(&cfg).await.expect("migrate");
    assert_eq!(out.exit_code, 0, "migrate must succeed");

    let adopted = state_smoke_conn::count_audit_rows(&mut conn, "object", "adopted")
        .await
        .expect("adopted count");
    assert_eq!(adopted, 1, "exactly the pre-created object is adopted");

    state_smoke::assert_smoke_objects_materialized(&mut conn)
        .await
        .expect("remaining smoke objects created");

    // Adoption recorded the checksum baseline → warm plan skips unchanged.
    let (warm, _) = engine_smoke::plan(&cfg).await.expect("warm plan");
    assert_eq!(
        smoke_table_action(&warm),
        Action::SkipUnchanged,
        "adopted object must be skipped on re-plan"
    );

    eprintln!("adopt_e2e OK: adopted={adopted}");
}

fn smoke_table_action(plan: &MigrationPlan) -> Action {
    plan.objects
        .iter()
        .find(|o| o.kind.as_ref() == "tables" && o.normalized_key.ends_with("smoke_table"))
        .map(|o| o.planned_action)
        .expect("smoke_table in plan")
}
