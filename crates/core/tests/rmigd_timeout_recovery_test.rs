//! Live regression for cleanup after an `rmigd` command timeout.

use std::time::Duration;

use migrator_core::config::validate_config;
use migrator_core::session::ProxyClient;
use migrator_core::Config;

fn repo_root() -> std::path::PathBuf {
    std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("../..")
        .canonicalize()
        .expect("repo root")
}

#[path = "common/rmigd.rs"]
mod rmigd;

fn enabled() -> bool {
    matches!(
        std::env::var("RMIG_RUN_SQLSERVER_INTEGRATION").as_deref(),
        Ok("1") | Ok("true")
    ) && matches!(
        std::env::var("RMIG_USE_RMIGD").as_deref(),
        Ok("1") | Ok("true") | Ok("yes")
    )
}

fn config(socket: String) -> Config {
    let mut cfg = Config::default();
    cfg.server = std::env::var("RM_DB_SERVER").unwrap_or_else(|_| "localhost".into());
    cfg.port = std::env::var("RM_DB_PORT").unwrap_or_else(|_| "1433".into());
    cfg.user = std::env::var("RM_DB_USER").unwrap_or_else(|_| "sa".into());
    cfg.password =
        std::env::var("RM_DB_PASSWORD").unwrap_or_else(|_| "yourStrong(!)Password".into());
    cfg.database = std::env::var("RM_DB_DATABASE").unwrap_or_else(|_| "master".into());
    cfg.sql_root = ".".into();
    cfg.sql_base = ".".into();
    cfg.session_socket = socket;
    cfg.session_token = std::env::var("RMIG_SESSION_TOKEN").expect("rmigd test token");
    cfg.command_timeout = Duration::from_secs(8);
    cfg.skip_git = true;
    // The checked-in Docker SQL Server fixture has no TLS endpoint.
    cfg.encrypt = false;
    cfg.trust_server_certificate = true;
    validate_config(&mut cfg).expect("valid rmigd test config");
    cfg
}

#[tokio::test]
async fn timed_out_transaction_and_session_lock_are_cleaned_before_reuse() {
    if !enabled() {
        return;
    }

    // This process owns the daemon, so pin its server-side request timeout.
    std::env::set_var("RM_COMMAND_TIMEOUT", "1s");
    let socket = rmigd::ensure_started().expect("rmigd enabled");
    let cfg = config(socket.clone());

    let mut timed_out = ProxyClient::connect(&socket, Some(&cfg))
        .await
        .expect("connect timeout client");
    let error = timed_out
        .exec(
            r#"BEGIN TRANSACTION;
               DECLARE @lock_result int;
               EXEC @lock_result = sys.sp_getapplock
                   @Resource = 'reporting_layer_migration',
                   @LockMode = 'Exclusive',
                   @LockOwner = 'Session',
                   @LockTimeout = 0;
               IF @lock_result < 0 THROW 51000, 'test lock failed', 1;
               WAITFOR DELAY '00:00:02';"#,
        )
        .await
        .expect_err("daemon request must time out");
    assert!(
        error.to_string().contains("rmigd: request timed out"),
        "unexpected timeout error: {error}"
    );
    drop(timed_out);

    let recovery = tokio::time::timeout(Duration::from_secs(8), async {
        let mut client = ProxyClient::connect(&socket, Some(&cfg)).await?;
        client
            .query(
                r#"DECLARE @lock_result int;
                   EXEC @lock_result = sys.sp_getapplock
                       @Resource = 'reporting_layer_migration',
                       @LockMode = 'Exclusive',
                       @LockOwner = 'Session',
                       @LockTimeout = 0;
                   IF @lock_result >= 0
                       EXEC sys.sp_releaseapplock
                           @Resource = 'reporting_layer_migration',
                           @LockOwner = 'Session';
                   SELECT CAST(@@TRANCOUNT AS int),
                          CAST(APPLOCK_MODE(
                              'public', 'reporting_layer_migration', 'Session'
                          ) AS nvarchar(32)),
                          CAST(1 AS int),
                          CAST(@lock_result AS int);"#,
                &[],
            )
            .await
    })
    .await
    .expect("daemon recovery must be bounded")
    .expect("daemon must serve the next client");

    let row = recovery.first().expect("recovery query row");
    assert_eq!(row.get_i32(0), Some(0), "transaction leaked after timeout");
    assert_eq!(
        row.get_str(1),
        Some("NoLock"),
        "session advisory lock leaked after timeout"
    );
    assert_eq!(row.get_i32(2), Some(1), "shared connection is poisoned");
    assert!(
        row.get_i32(3).is_some_and(|result| result >= 0),
        "timed-out SQL session still owns the advisory lock: {:?}",
        row.get_i32(3)
    );
}
