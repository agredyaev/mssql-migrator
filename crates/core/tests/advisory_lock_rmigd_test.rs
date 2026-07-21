fn repo_root() -> std::path::PathBuf {
    std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("../..")
        .canonicalize()
        .expect("repo root")
}

#[path = "common/rmigd.rs"]
mod rmigd;

use migrator_core::config::validate_config;
use migrator_core::driver::TimingConn;
use migrator_core::error::Error;
use migrator_core::lock::{acquire, release_after_body};
use migrator_core::session::connect_session_or_direct;
use migrator_core::Config;
use std::sync::{Arc, Mutex};

fn integration_enabled() -> bool {
    std::env::var("RMIG_RUN_SQLSERVER_INTEGRATION")
        .map(|v| v == "1" || v.eq_ignore_ascii_case("true"))
        .unwrap_or(false)
}

fn rmigd_enabled() -> bool {
    matches!(
        std::env::var("RMIG_USE_RMIGD").as_deref(),
        Ok("1") | Ok("true") | Ok("yes")
    )
}

fn parity_cfg(database: &str) -> Config {
    let mut cfg = Config::default();
    cfg.server = std::env::var("RM_DB_SERVER").unwrap_or_else(|_| "localhost".into());
    cfg.port = std::env::var("RM_DB_PORT").unwrap_or_else(|_| "1433".into());
    cfg.user = std::env::var("RM_DB_USER").unwrap_or_else(|_| "sa".into());
    cfg.password =
        std::env::var("RM_DB_PASSWORD").unwrap_or_else(|_| "yourStrong(!)Password".into());
    cfg.database = database.into();
    cfg.sql_root = ".".into();
    cfg.sql_base = ".".into();
    cfg.skip_git = true;
    cfg.encrypt = false;
    cfg.trust_server_certificate = true;
    if let Some(sock) = rmigd::ensure_started() {
        cfg.session_socket = sock;
        cfg.session_token = std::env::var("RMIG_SESSION_TOKEN")
            .expect("rmigd harness must publish its token through process environment");
    }
    validate_config(&mut cfg).expect("valid cfg");
    cfg
}

async fn with_advisory_lock<T>(
    conn: &mut TimingConn,
    cfg: &Config,
    body: impl std::future::Future<Output = migrator_core::error::Result<T>>,
) -> migrator_core::error::Result<T> {
    acquire(conn, cfg).await?;
    let body_result = body.await;
    release_after_body(conn, body_result).await
}

async fn timing_conn(cfg: &Config) -> TimingConn {
    let io = Arc::new(Mutex::new(migrator_core::driver::IoProfile::default()));
    TimingConn::new(
        connect_session_or_direct(cfg).await.expect("rmigd connect"),
        io,
    )
}

fn skip_unless_rmigd() -> bool {
    if !integration_enabled() || !rmigd_enabled() {
        return true;
    }
    rmigd::ensure_started().is_none()
}

#[tokio::test]
async fn rmigd_with_advisory_lock_releases_after_success_happy_path() {
    if skip_unless_rmigd() {
        return;
    }
    let cfg = parity_cfg("master");
    let mut holder = timing_conn(&cfg).await;
    with_advisory_lock(&mut holder, &cfg, async { Ok(()) })
        .await
        .expect("guard success");
    // Disconnect the holder so its exclusive daemon session (and the lock) is
    // released before the next client connects.
    drop(holder);
    let mut other = timing_conn(&cfg).await;
    acquire(&mut other, &cfg)
        .await
        .expect("lock must be free after rmigd guard success");
}

#[tokio::test]
async fn rmigd_with_advisory_lock_releases_after_body_error_negative_path() {
    if skip_unless_rmigd() {
        return;
    }
    let cfg = parity_cfg("master");
    let mut holder = timing_conn(&cfg).await;
    let err = with_advisory_lock(&mut holder, &cfg, async {
        Err::<(), _>(Error::Sql("simulated rmigd apply failure".into()))
    })
    .await
    .expect_err("body should fail");
    assert!(
        err.to_string().contains("simulated rmigd apply failure"),
        "unexpected error: {err}"
    );
    drop(holder);
    let mut other = timing_conn(&cfg).await;
    acquire(&mut other, &cfg)
        .await
        .expect("lock must be released after rmigd body error");
}

#[tokio::test]
async fn rmigd_serializes_concurrent_clients_no_reentrant_lock_edge_case() {
    if skip_unless_rmigd() {
        return;
    }
    let cfg = parity_cfg("master");
    // Holder connects and acquires; its daemon session now holds the exclusive
    // client lock for its lifetime.
    let mut holder = timing_conn(&cfg).await;
    acquire(&mut holder, &cfg).await.expect("first acquire");
    // A second client must NOT be able to reenter the shared session's advisory
    // lock while the holder is active — it blocks on the exclusive daemon session
    // (and would fall back to a direct connection that then contends on the real
    // DB applock). Prove it does not complete quickly.
    let blocked = tokio::time::timeout(std::time::Duration::from_secs(2), async {
        let mut contender = timing_conn(&cfg).await;
        with_advisory_lock(&mut contender, &cfg, async { Ok(()) }).await
    })
    .await;
    assert!(
        blocked.is_err(),
        "second client must be serialized behind the active session, not reenter the lock"
    );
    // After the holder releases and disconnects, the lock is free again.
    release_after_body(&mut holder, Ok::<(), Error>(()))
        .await
        .expect("release holder");
    drop(holder);
    let mut other = timing_conn(&cfg).await;
    acquire(&mut other, &cfg)
        .await
        .expect("lock free after holder releases and disconnects");
}

#[tokio::test]
async fn rmigd_with_advisory_lock_releases_after_failed_body_regression() {
    if skip_unless_rmigd() {
        return;
    }
    let cfg = parity_cfg("master");
    let mut holder = timing_conn(&cfg).await;
    with_advisory_lock(&mut holder, &cfg, async {
        Err::<(), _>(Error::Sql("BG-001 rmigd regression apply failure".into()))
    })
    .await
    .expect_err("BG-001 rmigd regression body");
    drop(holder);
    let mut other = timing_conn(&cfg).await;
    acquire(&mut other, &cfg)
        .await
        .expect("BG-001 regression: rmigd warm session must not keep lock after failed apply body");
}
