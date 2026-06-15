use tiberius::{Client, Config as TdsConfig};
use tokio::net::TcpStream;
use tokio_util::compat::{Compat, TokioAsyncWriteCompatExt};

use crate::config::Config;
use crate::error::{Error, Result};

use super::mssql_auth::select_auth_method;

pub type RawClient = Client<Compat<TcpStream>>;

pub struct MssqlConn {
    pub client: RawClient,
}

// Re-export query helpers so crate::driver::mssql::exec etc still resolve.
pub use super::mssql_query::{exec, ping, query_all_results, query_tiberius};

pub async fn connect(cfg: &Config) -> Result<MssqlConn> {
    let mut tds = TdsConfig::new();
    tds.host(&cfg.server);
    if let Ok(port) = cfg.port.parse::<u16>() {
        tds.port(port);
    }
    tds.database(&cfg.database);
    tds.authentication(select_auth_method(cfg)?);
    if cfg.trust_server_certificate() {
        tds.trust_cert();
    }
    if cfg.encrypt() {
        tds.encryption(tiberius::EncryptionLevel::Required);
    } else {
        tds.encryption(tiberius::EncryptionLevel::NotSupported);
    }
    let addr = format!("{}:{}", cfg.server, cfg.port);
    let tcp = match TcpStream::connect(&addr).await {
        Ok(tcp) => tcp,
        Err(e) => {
            tracing::debug!(
                operation = "tcp_connect",
                server = %cfg.server,
                port = %cfg.port,
                database = %cfg.database,
                db_auth = %cfg.db_auth,
                error = %e,
                "sql server tcp connect failed"
            );
            return Err(Error::Sql(format!("connect {addr}: {e}")));
        }
    };
    tcp.set_nodelay(true).ok();
    let mut client = match Client::connect(tds, tcp.compat_write()).await {
        Ok(client) => client,
        Err(e) => {
            tracing::debug!(
                operation = "tds_handshake",
                server = %cfg.server,
                port = %cfg.port,
                database = %cfg.database,
                db_auth = %cfg.db_auth,
                error = %e,
                "sql server tds handshake failed"
            );
            return Err(Error::Sql(format!("connect {addr}: tds handshake: {e}")));
        }
    };
    init_session(&mut client).await?;
    Ok(MssqlConn { client })
}

async fn init_session(client: &mut RawClient) -> Result<()> {
    super::mssql_query::exec(
        client,
        "SET QUOTED_IDENTIFIER ON; SET ANSI_NULLS ON; SET ANSI_PADDING ON;",
    )
    .await
}

#[cfg(test)]
#[path = "../tests/driver_mssql_test.rs"]
mod driver_mssql_tests;
