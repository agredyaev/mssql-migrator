use crate::driver::DbClient;
use crate::error::Result;

/// Open a CLI connection via `rmigd` (warm TDS held in the daemon process).
pub async fn connect_daemon(socket_path: &str, database: &str) -> Result<DbClient> {
    let mut proxy = super::proxy::ProxyClient::connect(socket_path).await?;
    if !database.is_empty() {
        let escaped = database.replace(']', "]]");
        proxy.exec(&format!("USE [{escaped}]")).await?;
    }
    Ok(DbClient::Proxy(proxy))
}
