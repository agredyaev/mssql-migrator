//! Drift-lifecycle e2e pins: out-of-band drop/modify, failed-apply batching,
//! and audit-history robustness (duplicate rows, zero checksums, repair).
//!
//! These pin the CURRENT engine semantics so later drift-detection work cannot
//! silently change them: plan compares repo checksums against recorded history
//! only — the live object body is never inspected.
//!
//! Run:
//!   RMIG_RUN_SQLSERVER_INTEGRATION=1 cargo test -p migrator-core --test drift_e2e_integration -- --nocapture --test-threads=1

#[path = "common/integration_enabled.rs"]
mod integration_enabled;

#[path = "common/workflow_config.rs"]
mod workflow_config;

#[path = "common/db_reset.rs"]
mod db_reset;

#[path = "common/state_smoke_conn.rs"]
mod state_smoke_conn;

#[path = "common/engine_smoke.rs"]
mod engine_smoke;

use migrator_core::config::Config;
use migrator_core::domain::Action;
use migrator_core::driver::TimingConn;
use migrator_core::engine::{run_command, Command, RunOutput};
use migrator_core::export::MigrationPlan;

const VIEW_KEY: &str = "smoke/views/smoke_view";
const BOGUS_CHECKSUM: &str = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef";

/// Out-of-band DROP is healed by re-create; out-of-band MODIFY is silently
/// skipped (known blindness: live bodies are never compared).
#[tokio::test(flavor = "current_thread")]
async fn oob_drop_recreated_and_oob_modify_silently_skipped() {
    if !integration_enabled::enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }
    let cfg = fresh_cold_db().await;
    let mut conn = state_smoke_conn::open_conn(&cfg).await.expect("connect");

    assert_eq!(migrate(&cfg).await, 0, "cold migrate");

    // Out-of-band drop: history says applied, DB says gone → plan re-creates.
    conn.exec("DROP VIEW smoke.smoke_view")
        .await
        .expect("oob drop view");
    assert_eq!(
        view_action(&cfg).await,
        Action::CreateObject,
        "after oob drop"
    );
    assert_eq!(migrate(&cfg).await, 0, "re-create migrate");
    let applied = count_key_rows(&mut conn, VIEW_KEY, "applied").await;
    assert_eq!(applied, 2, "one applied row per create");
    assert_eq!(view_action(&cfg).await, Action::SkipUnchanged, "healed");

    // Out-of-band modify: latest history checksum still equals the repo file,
    // so the drifted live body is skipped silently and survives the migrate.
    conn.exec("CREATE OR ALTER VIEW smoke.smoke_view AS SELECT CAST(99 AS INT) AS drifted")
        .await
        .expect("oob modify view");
    let plan = fresh_plan(&cfg).await;
    assert_eq!(
        action_of(&plan),
        Action::SkipUnchanged,
        "oob modify undetected"
    );
    assert!(!plan.blocked, "oob modify must not block");
    assert_eq!(migrate(&cfg).await, 0, "migrate over drifted body");
    conn.query("SELECT drifted FROM smoke.smoke_view", &[])
        .await
        .expect("drifted body must survive the migrate untouched");

    eprintln!("drift_e2e oob OK: applied_rows={applied}");
}

/// A failing module writes no history row, halts the module batch, and a
/// retry after the fix applies exactly once.
#[tokio::test(flavor = "current_thread")]
async fn failed_apply_halts_batch_no_history_then_retry_applies_once() {
    if !integration_enabled::enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }
    let cfg = fresh_cold_db().await;
    let mut conn = state_smoke_conn::open_conn(&cfg).await.expect("connect");

    // Kind collision: a TABLE occupies the view's name, so the repo's
    // CREATE OR ALTER VIEW fails at exec time without touching the repo tree.
    conn.exec("IF SCHEMA_ID(N'smoke') IS NULL EXEC('CREATE SCHEMA [smoke]')")
        .await
        .expect("create smoke schema");
    conn.exec("CREATE TABLE smoke.smoke_view (id INT NOT NULL)")
        .await
        .expect("create colliding table");

    assert_ne!(migrate(&cfg).await, 0, "migrate must fail on the collision");
    assert_eq!(
        count_key_rows(&mut conn, VIEW_KEY, "applied").await,
        0,
        "failed module must write no history row"
    );
    // Modules apply in kind order (views → functions → procedures): the view
    // failure must halt the batch before the procedure is created.
    let proc_created = object_exists(&mut conn, "refresh_smoke").await;
    assert!(!proc_created, "batch must halt at first module failure");

    // Fix (drop the collision) → retry applies exactly once.
    conn.exec("DROP TABLE smoke.smoke_view")
        .await
        .expect("drop colliding table");
    assert_eq!(migrate(&cfg).await, 0, "retry migrate");
    assert_eq!(
        count_key_rows(&mut conn, VIEW_KEY, "applied").await,
        1,
        "retry applies exactly once"
    );
    assert!(
        object_exists(&mut conn, "refresh_smoke").await,
        "batch resumes past the fixed module"
    );

    eprintln!("drift_e2e fail/retry OK");
}

/// History robustness: duplicate rows resolve latest-wins, a zero checksum
/// re-adopts, and repair-checksum recovers a corrupted row.
#[tokio::test(flavor = "current_thread")]
async fn history_rows_latest_wins_zero_readopts_and_repair_recovers() {
    if !integration_enabled::enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }
    let cfg = fresh_cold_db().await;
    let mut conn = state_smoke_conn::open_conn(&cfg).await.expect("connect");
    assert_eq!(migrate(&cfg).await, 0, "cold migrate");

    let good = top_checksum(&mut conn, VIEW_KEY).await;
    assert_eq!(good.len(), 64, "stored checksum is 64-hex");

    // Duplicate rows: the highest id wins.
    insert_history(&mut conn, VIEW_KEY, BOGUS_CHECKSUM, "applied").await;
    assert_eq!(
        view_action(&cfg).await,
        Action::UpdateExistingModule,
        "bogus latest"
    );
    insert_history(&mut conn, VIEW_KEY, &good, "applied").await;
    assert_eq!(
        view_action(&cfg).await,
        Action::SkipUnchanged,
        "good latest wins"
    );

    // Zero checksum parses as no-baseline → re-adopt.
    insert_history(&mut conn, VIEW_KEY, "", "applied").await;
    assert_eq!(
        view_action(&cfg).await,
        Action::AdoptExisting,
        "zero checksum readopts"
    );
    assert_eq!(migrate(&cfg).await, 0, "adopt migrate");
    assert_eq!(
        count_key_rows(&mut conn, VIEW_KEY, "adopted").await,
        1,
        "adoption recorded"
    );
    assert_eq!(
        view_action(&cfg).await,
        Action::SkipUnchanged,
        "clean after adopt"
    );

    // Corrupted latest row → repair-checksum re-applies and recovers.
    conn.exec(&format!(
        "UPDATE azdo_deploy_meta.history SET checksum = '{BOGUS_CHECKSUM}' \
         WHERE id = (SELECT MAX(id) FROM azdo_deploy_meta.history \
         WHERE normalized_key = '{VIEW_KEY}' AND kind = 'object' \
         AND event IN ('applied','adopted'))"
    ))
    .await
    .expect("corrupt latest row");
    assert_eq!(
        view_action(&cfg).await,
        Action::UpdateExistingModule,
        "corruption visible"
    );
    let out = repair(&cfg).await.expect("repair-checksum");
    assert_eq!(out.exit_code, 0, "repair-checksum must succeed");
    assert_eq!(view_action(&cfg).await, Action::SkipUnchanged, "repaired");

    eprintln!("drift_e2e history OK");
}

async fn fresh_cold_db() -> Config {
    let mut cfg = workflow_config::workflow_config().clone();
    cfg.set_skip_git(true);
    // These pins exercise the diff engine, not the cache layers: the DB
    // catalog_cache would otherwise mask out-of-band changes (hazard H2).
    cfg.set_catalog_cache(false);
    db_reset::reset_test_database(&cfg).await.expect("reset db");
    cfg
}

/// Every test mutates DB state behind the engine's back; a real deployment
/// runs one process per rmig invocation, so drop the in-process caches to
/// match that lifecycle before the next plan/migrate.
async fn oob_barrier(cfg: &Config) {
    db_reset::invalidate_process_caches(cfg, true)
        .await
        .expect("invalidate process caches");
}

async fn migrate(cfg: &Config) -> i32 {
    oob_barrier(cfg).await;
    engine_smoke::baseline_migrate(cfg)
        .await
        .map(|o| o.exit_code)
        .unwrap_or(-1)
}

async fn repair(cfg: &Config) -> migrator_core::error::Result<RunOutput> {
    oob_barrier(cfg).await;
    let mut c = cfg.clone();
    c.set_skip_git(true);
    c.session_socket.clear();
    run_command(Command::RepairChecksum, &c).await
}

async fn fresh_plan(cfg: &Config) -> MigrationPlan {
    oob_barrier(cfg).await;
    let (plan, _) = engine_smoke::plan(cfg).await.expect("plan");
    plan
}

async fn view_action(cfg: &Config) -> Action {
    action_of(&fresh_plan(cfg).await)
}

fn action_of(plan: &MigrationPlan) -> Action {
    plan.objects
        .iter()
        .find(|o| o.normalized_key.as_ref() == VIEW_KEY)
        .map(|o| o.planned_action)
        .expect("smoke_view in plan")
}

async fn count_key_rows(conn: &mut TimingConn, key: &str, event: &str) -> i32 {
    let rows = conn
        .query(
            "SELECT COUNT(*) FROM azdo_deploy_meta.history \
             WHERE normalized_key = @p1 AND kind = 'object' AND event = @p2",
            &[key, event],
        )
        .await
        .expect("history count");
    rows.first().and_then(|r| r.get_i32(0)).unwrap_or(0)
}

async fn top_checksum(conn: &mut TimingConn, key: &str) -> String {
    let rows = conn
        .query(
            "SELECT TOP 1 checksum FROM azdo_deploy_meta.history \
             WHERE normalized_key = @p1 AND kind = 'object' \
             AND event IN ('applied','adopted') ORDER BY id DESC",
            &[key],
        )
        .await
        .expect("top checksum");
    rows.first()
        .and_then(|r| r.get_str(0))
        .expect("checksum present")
        .to_string()
}

async fn insert_history(conn: &mut TimingConn, key: &str, checksum: &str, event: &str) {
    conn.exec(&format!(
        "INSERT INTO azdo_deploy_meta.history \
         (normalized_key, kind, checksum, git_hash, git_author, git_date, event, created_at) \
         VALUES ('{key}', 'object', '{checksum}', '', '', '1900-01-01', '{event}', SYSUTCDATETIME())"
    ))
    .await
    .expect("insert history row");
}

async fn object_exists(conn: &mut TimingConn, name: &str) -> bool {
    let rows = conn
        .query(
            "SELECT COUNT(*) FROM sys.objects o \
             INNER JOIN sys.schemas s ON s.schema_id = o.schema_id \
             WHERE s.name = 'smoke' AND o.name = @p1",
            &[name],
        )
        .await
        .expect("object probe");
    rows.first().and_then(|r| r.get_i32(0)).unwrap_or(0) > 0
}
