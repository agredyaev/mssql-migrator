use crate::config::Config;
use crate::driver::{connect, DbClient};
use crate::error::Result;

/// Open a CLI connection via `rmigd` (warm TDS held in the daemon process).
pub async fn connect_daemon(socket_path: &str, cfg: &Config) -> Result<DbClient> {
    let mut proxy = super::proxy::ProxyClient::connect(socket_path, cfg).await?;
    if !cfg.database.is_empty() {
        let quoted = crate::sql_ident::bracket_ident(&cfg.database)?;
        let use_stmt = format!("USE {quoted}");
        // The USE runs after ProxyClient::connect's bounded handshake but
        // before TimingConn's per-command timeout exists — an unbounded await
        // here lets a wedged daemon hang every CLI run.
        let t = cfg.command_timeout;
        if t.is_zero() {
            proxy.exec(&use_stmt).await?;
        } else {
            tokio::time::timeout(t, proxy.exec(&use_stmt))
                .await
                .map_err(|_| {
                    crate::error::Error::Config(format!(
                        "rmigd session init (USE) timed out after {t:?}"
                    ))
                })??;
        }
    }
    Ok(DbClient::Proxy(proxy))
}

/// Connect through `rmigd` when `RMIG_SESSION` is set; fall back to direct TDS on socket failure.
pub async fn connect_session_or_direct(cfg: &Config) -> Result<DbClient> {
    if cfg.session_socket.is_empty() {
        return connect_direct_without_session(cfg).await;
    }

    match connect_daemon(&cfg.session_socket, cfg).await {
        Ok(client) => Ok(client),
        Err(err) => connect_direct_after_daemon_error(cfg, err).await,
    }
}

async fn connect_direct_without_session(cfg: &Config) -> Result<DbClient> {
    tracing::debug!(
        database = %cfg.database,
        "session socket empty; using direct sql connection"
    );
    Ok(DbClient::Direct(connect(cfg).await?.client))
}

async fn connect_direct_after_daemon_error(
    cfg: &Config,
    daemon_err: crate::error::Error,
) -> Result<DbClient> {
    tracing::warn!(
        session_socket = %cfg.session_socket,
        database = %cfg.database,
        error = %daemon_err,
        "rmigd unavailable; falling back to direct sql connection"
    );
    connect(cfg)
        .await
        .map(|conn| DbClient::Direct(conn.client))
        .map_err(|err| {
            tracing::warn!(
                session_socket = %cfg.session_socket,
                database = %cfg.database,
                error = %err,
                "direct sql fallback failed after rmigd error"
            );
            err
        })
}
