use migrator_core::config::validate_config;
use migrator_core::driver::{connect, DbClient, TimingConn};
use migrator_core::error::Error;
use migrator_core::lock::{acquire, release_after_body};
use migrator_core::Config;
use std::sync::{Arc, Mutex};

fn integration_enabled() -> bool {
    std::env::var("RMIG_RUN_SQLSERVER_INTEGRATION")
        .map(|v| v == "1" || v.eq_ignore_ascii_case("true"))
        .unwrap_or(false)
}

fn parity_cfg(database: &str) -> Config {
    let mut cfg = Config {
        server: std::env::var("RM_DB_SERVER").unwrap_or_else(|_| "localhost".into()),
        port: std::env::var("RM_DB_PORT").unwrap_or_else(|_| "1433".into()),
        user: std::env::var("RM_DB_USER").unwrap_or_else(|_| "sa".into()),
        password: std::env::var("RM_DB_PASSWORD")
            .unwrap_or_else(|_| "yourStrong(!)Password".into()),
        database: database.into(),
        sql_root: ".".into(),
        sql_base: ".".into(),
        skip_git: true,
        encrypt: false,
        trust_server_certificate: true,
        ..Default::default()
    };
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
        DbClient::Direct(connect(cfg).await.expect("connect").client),
        io,
    )
}

#[tokio::test]
async fn with_advisory_lock_releases_after_success_happy_path() {
    if !integration_enabled() {
        return;
    }
    let cfg = parity_cfg("master");
    let mut holder = timing_conn(&cfg).await;
    with_advisory_lock(&mut holder, &cfg, async { Ok(()) })
        .await
        .expect("guard success");
    let mut other = timing_conn(&cfg).await;
    acquire(&mut other, &cfg)
        .await
        .expect("lock must be free after guard success");
}

#[tokio::test]
async fn with_advisory_lock_releases_after_body_error_negative_path() {
    if !integration_enabled() {
        return;
    }
    let cfg = parity_cfg("master");
    let mut holder = timing_conn(&cfg).await;
    let err = with_advisory_lock(&mut holder, &cfg, async {
        Err::<(), _>(Error::Sql("simulated apply failure".into()))
    })
    .await
    .expect_err("body should fail");
    assert!(
        err.to_string().contains("simulated apply failure"),
        "unexpected error: {err}"
    );
    let mut other = timing_conn(&cfg).await;
    acquire(&mut other, &cfg)
        .await
        .expect("lock must be released after body error");
}

#[tokio::test]
async fn with_advisory_lock_propagates_acquire_timeout_edge_case() {
    if !integration_enabled() {
        return;
    }
    let mut cfg = parity_cfg("master");
    cfg.lock_timeout = std::time::Duration::from_millis(1);
    let mut holder = timing_conn(&cfg).await;
    acquire(&mut holder, &cfg).await.expect("first acquire");
    let mut contender = timing_conn(&cfg).await;
    let err = with_advisory_lock(&mut contender, &cfg, async { Ok(()) })
        .await
        .expect_err("second acquire should time out");
    assert!(
        matches!(err, Error::LockTimeout),
        "expected lock timeout, got {err}"
    );
}

#[tokio::test]
async fn with_advisory_lock_releases_after_failed_body_regression() {
    if !integration_enabled() {
        return;
    }
    let cfg = parity_cfg("master");
    let mut holder = timing_conn(&cfg).await;
    with_advisory_lock(&mut holder, &cfg, async {
        Err::<(), _>(Error::Sql("BG-001 regression apply failure".into()))
    })
    .await
    .expect_err("BG-001 regression body");
    let mut other = timing_conn(&cfg).await;
    acquire(&mut other, &cfg).await.expect(
        "BG-001 regression: daemon/direct session must not keep lock after failed apply body",
    );
}
