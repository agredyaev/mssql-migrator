use crate::config::Config;
use crate::driver::{connect, DbClient};
use crate::error::Result;

/// Open a CLI connection via `rmigd` (warm TDS held in the daemon process).
pub async fn connect_daemon(socket_path: &str, cfg: &Config) -> Result<DbClient> {
    Ok(DbClient::Proxy(
        super::proxy::ProxyClient::connect(socket_path, Some(cfg)).await?,
    ))
}

/// Connect through `rmigd` when `RMIG_SESSION` is set; fall back to direct TDS on socket failure.
pub async fn connect_session_or_direct(cfg: &Config) -> Result<DbClient> {
    if cfg.session_socket.is_empty() {
        tracing::debug!(
            database = %cfg.database,
            db_auth = %cfg.db_auth,
            "session socket empty; using direct sql connection"
        );
        return Ok(DbClient::Direct(connect(cfg).await?.client));
    }
    match connect_daemon(&cfg.session_socket, cfg).await {
        Ok(client) => Ok(client),
        Err(e) => {
            tracing::warn!(
                session_socket = %cfg.session_socket,
                database = %cfg.database,
                db_auth = %cfg.db_auth,
                error = %e,
                "rmigd unavailable; falling back to direct sql connection"
            );
            match connect(cfg).await {
                Ok(conn) => Ok(DbClient::Direct(conn.client)),
                Err(err) => {
                    tracing::warn!(
                        session_socket = %cfg.session_socket,
                        database = %cfg.database,
                        db_auth = %cfg.db_auth,
                        error = %err,
                        "direct sql fallback failed after rmigd error"
                    );
                    Err(err)
                }
            }
        }
    }
}
