use crate::driver::DbClient;
use crate::error::Result;

/// Open a CLI connection via `rmigd` (warm TDS held in the daemon process).
pub async fn connect_daemon(socket_path: &str) -> Result<DbClient> {
    Ok(DbClient::Proxy(
        super::proxy::ProxyClient::connect(socket_path).await?,
    ))
}
