//! Incremental drift tracking (Tier 2): the DDL trigger + object_ddl let a plan
//! fingerprint only objects changed since their last apply, while the read path
//! falls back to full fingerprinting when the trigger is absent or disabled, so
//! out-of-band drift is never missed.
//!
//! Run:
//!   RMIG_RUN_SQLSERVER_INTEGRATION=1 cargo test -p migrator-core \
//!     --test incremental_drift_test -- --nocapture --test-threads=1

#[path = "common/db_reset.rs"]
mod db_reset;
#[path = "common/engine_smoke.rs"]
mod engine_smoke;
#[path = "common/integration_enabled.rs"]
mod integration_enabled;
#[path = "common/state_smoke_conn.rs"]
mod state_smoke_conn;
#[path = "common/workflow_config.rs"]
mod workflow_config;

use migrator_core::config::Config;
use migrator_core::driver::TimingConn;

async fn fresh_cold_db() -> Config {
    let mut cfg = workflow_config::workflow_config().clone();
    cfg.set_skip_git(true);
    // Out-of-band drift must reach the diff engine, not be masked by a cache.
    cfg.set_catalog_cache(false);
    db_reset::reset_test_database(&cfg).await.expect("reset db");
    cfg
}

async fn barrier(cfg: &Config) {
    db_reset::invalidate_process_caches(cfg, true)
        .await
        .expect("invalidate process caches");
}

async fn scalar_i32(conn: &mut TimingConn, sql: &str) -> i32 {
    conn.query(sql, &[])
        .await
        .expect("query")
        .first()
        .and_then(|r| r.get_i32(0))
        .unwrap_or(-1)
}

/// After a cold migrate the DDL trigger and object_ddl versions exist, an
/// unchanged catalog plans clean, and an out-of-band structural change made
/// while the trigger is disabled is still blocked (force-fingerprint fallback).
#[tokio::test(flavor = "current_thread")]
async fn tracking_installed_and_disabled_trigger_still_catches_drift() {
    if !integration_enabled::enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }
    let cfg = fresh_cold_db().await;
    let mut conn = state_smoke_conn::open_conn(&cfg).await.expect("connect");

    barrier(&cfg).await;
    assert_eq!(
        engine_smoke::baseline_migrate(&cfg)
            .await
            .map(|o| o.exit_code)
            .unwrap_or(-1),
        0,
        "cold migrate"
    );

    // Tracking installed and populated by the migrator's own applies.
    assert_eq!(
        scalar_i32(
            &mut conn,
            "SELECT COUNT(*) FROM sys.triggers WHERE parent_class = 0 \
             AND name = 'azdo_deploy_meta_ddl_watch'",
        )
        .await,
        1,
        "database DDL trigger installed"
    );
    assert!(
        scalar_i32(
            &mut conn,
            "SELECT COUNT(*) FROM azdo_deploy_meta.object_ddl"
        )
        .await
            > 0,
        "object_ddl populated"
    );
    assert_eq!(
        scalar_i32(
            &mut conn,
            "SELECT COUNT(*) FROM azdo_deploy_meta.history \
             WHERE applied_ddl_version IS NULL AND event IN ('applied', 'adopted')",
        )
        .await,
        0,
        "every applied object recorded a ddl version"
    );

    // No false positive: an unchanged catalog is not blocked.
    barrier(&cfg).await;
    let (clean, _) = engine_smoke::plan(&cfg).await.expect("clean plan");
    assert!(!clean.blocked, "unchanged catalog must plan clean");

    // Fallback: disable the trigger, then make an out-of-band structural change.
    // object_ddl is not bumped, but the disabled trigger forces full
    // fingerprinting, so the drift is still detected and blocks.
    conn.exec("DISABLE TRIGGER azdo_deploy_meta_ddl_watch ON DATABASE")
        .await
        .expect("disable trigger");
    conn.exec("ALTER TABLE smoke.smoke_table ADD sneaky int NULL")
        .await
        .expect("out-of-band alter");
    barrier(&cfg).await;
    let (drifted, _) = engine_smoke::plan(&cfg).await.expect("drift plan");
    assert!(
        drifted.blocked,
        "structural drift must block even when the trigger was disabled"
    );

    // Leave the shared test database clean for later test binaries: undo the
    // out-of-band change and re-enable the trigger.
    conn.exec("ALTER TABLE smoke.smoke_table DROP COLUMN sneaky")
        .await
        .expect("revert out-of-band alter");
    conn.exec("ENABLE TRIGGER azdo_deploy_meta_ddl_watch ON DATABASE")
        .await
        .expect("re-enable trigger");
}
