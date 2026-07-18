//! Chaos: SIGKILL `rmig migrate` while a non-idempotent transition is mid-flight,
//! then re-run to completion. Asserts the crash left NO partial commit and the
//! migration is applied EXACTLY once (the core crash-safety guarantee).
//!
//! Run: RMIG_RUN_SQLSERVER_INTEGRATION=1 cargo test -p migrator-core \
//!        --test chaos_kill_mid_apply_test -- --nocapture --test-threads=1

#[path = "common/db_reset.rs"]
mod db_reset;
#[path = "common/integration_enabled.rs"]
mod integration_enabled;
#[path = "common/state_smoke_conn.rs"]
mod state_smoke_conn;

use std::path::{Path, PathBuf};
use std::process::Command;
use std::time::Duration;

use migrator_core::config::{build_config, validate_config, TomlConfig};
use migrator_core::driver::TimingConn;
use migrator_core::error::Result;
use migrator_core::Config;

const DB: &str = "chaos_kill_test";
const INS: &str = "INSERT INTO smoke.chaos (id) VALUES (1);\n";

fn table_sql(v: u32) -> String {
    format!("-- rev {v}\nCREATE TABLE smoke.chaos (id INT NOT NULL);\n")
}

fn repo_root() -> PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("../..")
        .canonicalize()
        .expect("repo root")
}

fn rmig_bin() -> PathBuf {
    // Always build: an existence-only check can run a stale binary against
    // current sources; cargo no-ops when the build is fresh.
    let ok = Command::new("cargo")
        .args(["build", "--release", "-p", "rmig"])
        .current_dir(repo_root())
        .status()
        .expect("build rmig")
        .success();
    assert!(ok, "rmig build failed");
    repo_root().join("target/release/rmig")
}

fn tables_dir(root: &Path) -> PathBuf {
    root.join(DB).join("smoke").join("tables")
}

fn write_table(root: &Path, rev: u32) {
    let base = tables_dir(root);
    std::fs::create_dir_all(&base).expect("mkdir");
    std::fs::write(base.join("chaos.sql"), table_sql(rev)).expect("table sql");
}

/// Write transition `001` (`waitfor_secs`=0 ⇒ fast). With a delay, the INSERT
/// runs FIRST so its uncommitted row is a database-visible (NOLOCK) barrier
/// proving the transaction started, then WAITFOR holds it open for the kill.
fn write_transition(root: &Path, waitfor_secs: u32) {
    let migr = tables_dir(root).join("_migrations").join("chaos");
    std::fs::create_dir_all(&migr).expect("mkdir");
    let body = if waitfor_secs > 0 {
        format!("{INS}WAITFOR DELAY '00:00:{waitfor_secs:02}';\n")
    } else {
        INS.to_string()
    };
    std::fs::write(migr.join("001_abcdef1_ins.sql"), body).expect("transition sql");
}

fn migrate_cmd(bin: &Path, root: &Path) -> Command {
    let mut c = Command::new(bin);
    c.arg("migrate")
        .env("RM_SQL_ROOT", root)
        .env("RM_SQL_BASE", root)
        .env(
            "RM_DB_SERVER",
            std::env::var("RM_DB_SERVER").unwrap_or_else(|_| "localhost".into()),
        )
        .env(
            "RM_DB_PORT",
            std::env::var("RM_DB_PORT").unwrap_or_else(|_| "1433".into()),
        )
        .env(
            "RM_DB_USER",
            std::env::var("RM_DB_USER").unwrap_or_else(|_| "sa".into()),
        )
        .env(
            "RM_DB_PASSWORD",
            std::env::var("RM_DB_PASSWORD").unwrap_or_else(|_| "yourStrong(!)Password".into()),
        )
        .env("RM_DB_TRUST_SERVER_CERTIFICATE", "true")
        .env("RM_SKIP_GIT", "1")
        .env("RMIG_USE_RMIGD", "0");
    c
}

fn chaos_cfg() -> Config {
    let mut cfg = build_config(&TomlConfig::default(), true);
    cfg.server = std::env::var("RM_DB_SERVER").unwrap_or_else(|_| "localhost".into());
    cfg.port = std::env::var("RM_DB_PORT").unwrap_or_else(|_| "1433".into());
    cfg.user = std::env::var("RM_DB_USER").unwrap_or_else(|_| "sa".into());
    cfg.password =
        std::env::var("RM_DB_PASSWORD").unwrap_or_else(|_| "yourStrong(!)Password".into());
    cfg.database = DB.into();
    cfg.sql_root = ".".into();
    cfg.set_skip_git(true);
    cfg.set_encrypt(false);
    cfg.set_trust_server_certificate(true);
    validate_config(&mut cfg).expect("valid cfg");
    cfg
}

async fn count(conn: &mut TimingConn, sql: &str) -> Result<i32> {
    let rows = conn.query(sql, &[]).await?;
    Ok(rows.first().and_then(|r| r.get_i32(0)).unwrap_or(0))
}

#[tokio::test(flavor = "multi_thread")]
async fn kill_mid_apply_no_partial_commit_and_reruns_exactly_once() {
    if !integration_enabled::enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }
    let cfg = chaos_cfg();
    // Ensure the DB exists so reset can DROP/CREATE cleanly.
    {
        let mut master = cfg.clone();
        master.database = "master".into();
        let mut c = migrator_core::driver::connect(&master)
            .await
            .expect("connect master");
        let _ = migrator_core::driver::mssql::exec(
            &mut c.client,
            &format!("IF DB_ID('{DB}') IS NULL CREATE DATABASE [{DB}]"),
        )
        .await;
    }
    db_reset::reset_test_database(&cfg).await.expect("reset db");

    let bin = rmig_bin();
    let dir = tempfile::tempdir().expect("tempdir");

    // --- Phase 1: cold migrate creates the table (no transitions run yet) ---
    write_table(dir.path(), 1);
    let s = migrate_cmd(&bin, dir.path())
        .status()
        .expect("phase1 migrate");
    assert!(s.success(), "phase1 cold migrate must succeed");

    // --- Phase 2: NEW transition on the (changed) table, killed mid-WAITFOR ---
    write_table(dir.path(), 2);
    write_transition(dir.path(), 10);
    let mut child = migrate_cmd(&bin, dir.path())
        .spawn()
        .expect("spawn migrate");
    // Barrier: wait until the transition transaction has demonstrably started
    // (its uncommitted INSERT is visible under NOLOCK) before killing. A fixed
    // sleep could kill during connect/planning, leaving the rollback guarantee
    // untested while every later assertion passes vacuously.
    let mut conn = state_smoke_conn::open_conn(&cfg).await.expect("connect");
    let started = std::time::Instant::now();
    loop {
        let uncommitted = count(&mut conn, "SELECT COUNT(*) FROM smoke.chaos WITH (NOLOCK)")
            .await
            .unwrap_or(0);
        if uncommitted > 0 {
            break;
        }
        assert!(
            started.elapsed() < Duration::from_secs(30),
            "transition transaction never became observable; cannot prove mid-transaction kill"
        );
        tokio::time::sleep(Duration::from_millis(100)).await;
    }
    child.kill().expect("kill migrate");
    let _ = child.wait();
    let rows_after_kill = count(&mut conn, "SELECT COUNT(*) FROM smoke.chaos")
        .await
        .expect("count rows");
    assert_eq!(
        rows_after_kill, 0,
        "killed transition must roll back — NO partial commit"
    );
    let migr_hist_after_kill =
        state_smoke_conn::count_audit_rows(&mut conn, "migration", "applied")
            .await
            .expect("audit");
    assert_eq!(
        migr_hist_after_kill, 0,
        "no history for a transition that never committed"
    );
    // The session applock must be free after the crashed connection closed:
    // acquiring it now must not block.
    let acquire = conn
        .query(migrator_core::sql::lock::ACQUIRE, &["2000"])
        .await
        .expect("acquire applock");
    assert!(
        acquire.first().and_then(|r| r.get_i32(0)).unwrap_or(-1) >= 0,
        "advisory lock must be free after the crash (session-scoped, freed on disconnect)"
    );
    let _ = conn.query(migrator_core::sql::lock::RELEASE, &[]).await;
    drop(conn);

    // --- Phase 3: fast transition, run to completion ---
    write_transition(dir.path(), 0);
    let status = migrate_cmd(&bin, dir.path())
        .status()
        .expect("run migrate to completion");
    assert!(
        status.success(),
        "re-run migrate must succeed, got {status:?}"
    );

    let mut conn = state_smoke_conn::open_conn(&cfg).await.expect("reconnect");
    let rows = count(&mut conn, "SELECT COUNT(*) FROM smoke.chaos")
        .await
        .expect("count rows");
    assert_eq!(
        rows, 1,
        "migration must be applied EXACTLY once — not zero, not twice"
    );
    let migr_hist = state_smoke_conn::count_audit_rows(&mut conn, "migration", "applied")
        .await
        .expect("audit");
    assert_eq!(migr_hist, 1, "exactly one migration history record");
}

async fn ensure_db_and_reset(cfg: &Config) {
    let mut master = cfg.clone();
    master.database = "master".into();
    let mut c = migrator_core::driver::connect(&master)
        .await
        .expect("connect master");
    let _ = migrator_core::driver::mssql::exec(
        &mut c.client,
        &format!("IF DB_ID('{DB}') IS NULL CREATE DATABASE [{DB}]"),
    )
    .await;
    db_reset::reset_test_database(cfg).await.expect("reset db");
}

/// Two `rmig migrate` racing on the SAME changed object (like two PRs merging at
/// once). The advisory lock must serialize them: the migration applies EXACTLY
/// once, never twice, and neither corrupts the other.
#[tokio::test(flavor = "multi_thread")]
async fn concurrent_migrate_same_object_serializes_exactly_once() {
    if !integration_enabled::enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }
    let cfg = chaos_cfg();
    ensure_db_and_reset(&cfg).await;

    let bin = rmig_bin();
    let dir = tempfile::tempdir().expect("tempdir");
    write_table(dir.path(), 1);
    assert!(
        migrate_cmd(&bin, dir.path())
            .status()
            .expect("phase0")
            .success(),
        "cold migrate must succeed"
    );

    // New transition on the changed table; a short WAITFOR widens the race so the
    // second process definitely contends on the advisory lock while the first holds it.
    write_table(dir.path(), 2);
    write_transition(dir.path(), 2);
    let a = migrate_cmd(&bin, dir.path()).spawn().expect("spawn a");
    let b = migrate_cmd(&bin, dir.path()).spawn().expect("spawn b");
    let sa = a.wait_with_output().expect("wait a");
    let sb = b.wait_with_output().expect("wait b");

    // Exit codes: 0 (applied or skip-unchanged) or 7 (lock timeout) are both
    // acceptable serializations; a crash/SQL error (other codes) is not.
    for (who, out) in [("A", &sa), ("B", &sb)] {
        let code = out.status.code().unwrap_or(-1);
        assert!(
            code == 0 || code == 7,
            "{who} exit {code} — expected 0 or 7 (lock timeout); stderr: {}",
            String::from_utf8_lossy(&out.stderr)
        );
    }
    assert!(
        sa.status.success() || sb.status.success(),
        "at least one racer must apply successfully"
    );

    let mut conn = state_smoke_conn::open_conn(&cfg).await.expect("connect");
    let rows = count(&mut conn, "SELECT COUNT(*) FROM smoke.chaos")
        .await
        .expect("count");
    assert_eq!(
        rows, 1,
        "concurrent racers must apply the INSERT EXACTLY once"
    );
    let migr_hist = state_smoke_conn::count_audit_rows(&mut conn, "migration", "applied")
        .await
        .expect("audit");
    assert_eq!(migr_hist, 1, "exactly one migration history record");
}
