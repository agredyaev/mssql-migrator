//! Apply-integrity e2e: a script body cannot escape the executor transaction,
//! `baseline`/`repair-checksum` never execute DDL, and implicit adoption during
//! `migrate` requires the explicit opt-in flag.
//!
//! Run:
//!   RMIG_RUN_SQLSERVER_INTEGRATION=1 cargo test -p migrator-core --test apply_integrity_integration -- --nocapture --test-threads=1

#[path = "common/integration_enabled.rs"]
mod integration_enabled;

#[path = "common/db_reset.rs"]
mod db_reset;

#[path = "common/state_smoke_conn.rs"]
mod state_smoke_conn;

use std::path::Path;

use migrator_core::config::{build_config, load_env_file, validate_config};
use migrator_core::domain::Action;
use migrator_core::driver::TimingConn;
use migrator_core::engine::{run_command, Command};
use migrator_core::error::EXIT_PLAN_BLOCKED;
use migrator_core::Config;

const DB: &str = "applyintegrity";
const TABLE_V1: &str = "CREATE TABLE smoke.guarded (id INT NOT NULL);\n";
const TABLE_V2: &str = "CREATE TABLE smoke.guarded (id INT NOT NULL, extra INT);\n";
const VIEW_V1: &str = "CREATE OR ALTER VIEW smoke.v_guard AS SELECT CAST(1 AS INT) AS original;\n";
const VIEW_V2: &str = "CREATE OR ALTER VIEW smoke.v_guard AS SELECT CAST(2 AS INT) AS changed;\n";

fn repo_root() -> std::path::PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("../..")
        .canonicalize()
        .expect("repo root")
}

fn test_cfg(sql_root: &Path) -> Config {
    let env = load_env_file(&repo_root().join(".env")).unwrap_or_default();
    let mut cfg = build_config(&env, true);
    if cfg.server.is_empty() {
        cfg.server = "127.0.0.1".into();
    }
    if cfg.user.is_empty() {
        cfg.user = "sa".into();
    }
    if cfg.password.is_empty() {
        cfg.password = "yourStrong(!)Password".into();
    }
    cfg.sql_root = sql_root.to_string_lossy().into();
    cfg.sql_base = cfg.sql_root.clone();
    cfg.set_skip_git(true);
    cfg.set_trust_server_certificate(true);
    cfg.set_catalog_cache(false);
    cfg.session_socket.clear();
    validate_config(&mut cfg).expect("valid config");
    cfg
}

fn write(root: &Path, rel: &str, body: &str) {
    let p = root.join(rel);
    std::fs::create_dir_all(p.parent().unwrap()).expect("mkdir");
    std::fs::write(p, body).expect("write fixture");
}

async fn fresh(root: &Path) -> (Config, TimingConn) {
    let cfg = test_cfg(root);
    assert_eq!(cfg.database, DB, "layout dir names the catalog database");
    db_reset::reset_test_database(&cfg).await.expect("reset db");
    let conn = state_smoke_conn::open_conn(&cfg).await.expect("connect");
    (cfg, conn)
}

async fn run(cfg: &Config, cmd: Command) -> migrator_core::error::Result<i32> {
    db_reset::invalidate_process_caches(cfg, true)
        .await
        .expect("invalidate caches");
    run_command(cmd, cfg).await.map(|o| o.exit_code)
}

async fn migration_rows(conn: &mut TimingConn, path_like: &str) -> i32 {
    let rows = conn
        .query(
            "SELECT COUNT(*) FROM azdo_deploy_meta.history WHERE normalized_key LIKE @p1",
            &[path_like],
        )
        .await
        .expect("history query");
    rows.first().and_then(|r| r.get_i32(0)).unwrap_or(0)
}

async fn object_id(conn: &mut TimingConn, name: &str) -> bool {
    let rows = conn
        .query("SELECT OBJECT_ID(@p1)", &[name])
        .await
        .expect("object_id");
    rows.first().and_then(|r| r.get_i32(0)).is_some()
}

/// A transition body that issues its own ROLLBACK must fail the migrate and
/// leave NO committed history row; the fixed script then applies exactly once.
#[tokio::test(flavor = "current_thread")]
async fn transition_rollback_body_cannot_commit_history_regression() {
    if !integration_enabled::enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }
    let dir = tempfile::tempdir().expect("tempdir");
    let root = dir.path();
    write(root, &format!("{DB}/smoke/tables/guarded.sql"), TABLE_V1);
    let (mut cfg, mut conn) = fresh(root).await;
    cfg.set_allow_adopt(true);
    assert_eq!(run(&cfg, Command::Migrate).await.expect("cold"), 0);

    // Change the table and add a transition whose body escapes the transaction.
    write(root, &format!("{DB}/smoke/tables/guarded.sql"), TABLE_V2);
    let trans = format!("{DB}/smoke/tables/_migrations/guarded/001_abcdef1_add.sql");
    write(root, &trans, "ROLLBACK TRANSACTION;\n");
    let err = run(&cfg, Command::Migrate)
        .await
        .expect_err("escaping body must fail the migrate");
    assert!(
        err.to_string().contains("transaction"),
        "error names the transaction escape: {err}"
    );
    assert_eq!(
        migration_rows(&mut conn, "%001_abcdef1_add.sql").await,
        0,
        "no history row may survive the failed transition"
    );

    // Fix the body → the transition is still pending and applies exactly once.
    write(root, &trans, "ALTER TABLE smoke.guarded ADD extra INT;\n");
    assert_eq!(run(&cfg, Command::Migrate).await.expect("retry"), 0);
    assert_eq!(
        migration_rows(&mut conn, "%001_abcdef1_add.sql").await,
        1,
        "fixed transition applies exactly once"
    );
}

/// `baseline` adopts existing objects only: it must never create missing ones,
/// and `repair-checksum` must never execute module DDL.
#[tokio::test(flavor = "current_thread")]
async fn baseline_and_repair_never_execute_ddl_regression() {
    if !integration_enabled::enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }
    let dir = tempfile::tempdir().expect("tempdir");
    let root = dir.path();
    write(root, &format!("{DB}/smoke/tables/guarded.sql"), TABLE_V1);
    write(root, &format!("{DB}/smoke/views/v_guard.sql"), VIEW_V1);
    let (cfg, mut conn) = fresh(root).await;

    // Pre-create ONLY the table (identical DDL); the view stays absent.
    conn.exec("IF SCHEMA_ID(N'smoke') IS NULL EXEC('CREATE SCHEMA [smoke]')")
        .await
        .expect("schema");
    conn.exec(TABLE_V1).await.expect("pre-create table");

    assert_eq!(run(&cfg, Command::Baseline).await.expect("baseline"), 0);
    assert!(
        !object_id(&mut conn, "smoke.v_guard").await,
        "baseline must not create the missing view"
    );
    assert_eq!(
        state_smoke_conn::count_audit_rows(&mut conn, "object", "adopted")
            .await
            .expect("adopted"),
        1,
        "baseline adopts exactly the pre-existing table"
    );

    // Migrate (with adoption satisfied) creates the view; then change the repo
    // body and prove repair-checksum re-baselines WITHOUT touching the live view.
    let mut allow = cfg.clone();
    allow.set_allow_adopt(true);
    assert_eq!(run(&allow, Command::Migrate).await.expect("migrate"), 0);
    write(root, &format!("{DB}/smoke/views/v_guard.sql"), VIEW_V2);
    assert_eq!(run(&cfg, Command::RepairChecksum).await.expect("repair"), 0);
    conn.query("SELECT original FROM smoke.v_guard", &[])
        .await
        .expect("live view must keep its ORIGINAL body after repair-checksum");
    let rows = conn
        .query(
            "SELECT TOP 1 checksum, event FROM azdo_deploy_meta.history \
             WHERE normalized_key = 'smoke/views/v_guard' ORDER BY id DESC",
            &[],
        )
        .await
        .expect("history probe");
    let latest = rows.first().expect("repair row present");
    assert_eq!(latest.get_str(1), Some("applied"), "repair row event");
    // sha256 of VIEW_V2 — the repository body at repair time.
    assert_eq!(
        latest.get_str(0),
        Some("d8c3d2e13c263d44d13b0a50b179011c3e26298225801d1ebe815ef2657a00b3"),
        "repair records the current repository checksum"
    );
    // One process per rmig invocation in production: drop in-process caches
    // before the verifying plan, like every other same-process e2e.
    db_reset::invalidate_process_caches(&cfg, true)
        .await
        .expect("invalidate caches");
    let out = run_command(Command::Plan, &cfg).await.expect("plan");
    let plan = out.plan.expect("plan present");
    let action = plan
        .objects
        .iter()
        .find(|o| o.normalized_key.as_ref() == "smoke/views/v_guard")
        .map(|o| o.planned_action)
        .expect("view in plan");
    assert_eq!(
        action,
        Action::SkipUnchanged,
        "repair re-baselined the view"
    );
}

/// Implicit adoption during `migrate` is refused without `RMIG_ALLOW_ADOPT`.
#[tokio::test(flavor = "current_thread")]
async fn migrate_blocks_implicit_adoption_without_flag_regression() {
    if !integration_enabled::enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }
    let dir = tempfile::tempdir().expect("tempdir");
    let root = dir.path();
    write(root, &format!("{DB}/smoke/tables/guarded.sql"), TABLE_V1);
    let (cfg, mut conn) = fresh(root).await;
    conn.exec("IF SCHEMA_ID(N'smoke') IS NULL EXEC('CREATE SCHEMA [smoke]')")
        .await
        .expect("schema");
    conn.exec(TABLE_V1).await.expect("pre-create table");

    let code = run(&cfg, Command::Migrate)
        .await
        .expect("blocked migrate still returns an exit code");
    assert_eq!(code, EXIT_PLAN_BLOCKED, "adoption without the flag blocks");
    assert_eq!(
        state_smoke_conn::count_audit_rows(&mut conn, "object", "adopted")
            .await
            .expect("adopted"),
        0,
        "nothing may be adopted while blocked"
    );

    let mut allow = cfg.clone();
    allow.set_allow_adopt(true);
    assert_eq!(run(&allow, Command::Migrate).await.expect("allowed"), 0);
    assert_eq!(
        state_smoke_conn::count_audit_rows(&mut conn, "object", "adopted")
            .await
            .expect("adopted"),
        1,
        "flagged migrate adopts the pre-existing object"
    );
}
