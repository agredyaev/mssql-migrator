//! Tiberius TDS client setup and [`MssqlConn`] connection wrapper.

use tiberius::{Client, Config as TdsConfig};
use tokio::net::TcpStream;
use tokio_util::compat::{Compat, TokioAsyncWriteCompatExt};

use crate::config::Config;
use crate::error::{Error, Result};

use super::mssql_auth::select_auth_method;

/// Tiberius client over a Tokio-compatible TCP stream.
pub type RawClient = Client<Compat<TcpStream>>;

/// Active MSSQL connection wrapping a Tiberius client.
pub struct MssqlConn {
    /// The underlying Tiberius client.
    pub client: RawClient,
}

// Re-export query helpers so crate::driver::mssql::exec etc still resolve.
pub use super::mssql_query::{exec, ping, query_all_results, query_tiberius};

/// Opens a new MSSQL connection, retrying a few times on transient
/// connection-phase failures (a one-off SYN drop or failover should not fail a
/// whole deploy). SQL/config errors are returned immediately, never retried.
pub async fn connect(cfg: &Config) -> Result<MssqlConn> {
    const MAX_ATTEMPTS: u32 = 3;
    let mut attempt = 1;
    loop {
        match connect_once(cfg).await {
            Ok(conn) => return Ok(conn),
            Err(Error::Conn(msg)) if attempt < MAX_ATTEMPTS => {
                tracing::warn!(attempt, server = %cfg.server, error = %msg, "connect failed; retrying");
                tokio::time::sleep(std::time::Duration::from_millis(500 * attempt as u64)).await;
                attempt += 1;
            }
            Err(e) => return Err(e),
        }
    }
}

async fn connect_once(cfg: &Config) -> Result<MssqlConn> {
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
    let timeout = cfg.command_timeout;
    let tcp = match with_connect_timeout(timeout, &addr, "tcp connect", TcpStream::connect(&addr))
        .await?
    {
        Ok(tcp) => tcp,
        Err(e) => {
            tracing::debug!(operation = "tcp_connect", server = %cfg.server, port = %cfg.port, database = %cfg.database, db_auth = %cfg.db_auth, error = %e, "sql server tcp connect failed");
            return Err(Error::Conn(format!("connect {addr}: {e}")));
        }
    };
    tcp.set_nodelay(true).ok();
    let handshake = Client::connect(tds, tcp.compat_write());
    let mut client = match with_connect_timeout(timeout, &addr, "tds handshake", handshake).await? {
        Ok(client) => client,
        Err(e) => {
            tracing::debug!(operation = "tds_handshake", server = %cfg.server, port = %cfg.port, database = %cfg.database, db_auth = %cfg.db_auth, error = %e, "sql server tds handshake failed");
            return Err(Error::Conn(format!("connect {addr}: tds handshake: {e}")));
        }
    };
    init_session(&mut client).await?;
    Ok(MssqlConn { client })
}

/// Bound a connect step (TCP dial / TDS handshake) by `timeout` so an unreachable
/// or wedged SQL Server fails fast instead of hanging CI. `Duration::ZERO` disables
/// the bound. On success the step's own result is returned unchanged.
async fn with_connect_timeout<F, T>(
    timeout: std::time::Duration,
    addr: &str,
    what: &str,
    fut: F,
) -> Result<T>
where
    F: std::future::Future<Output = T>,
{
    if timeout.is_zero() {
        return Ok(fut.await);
    }
    tokio::time::timeout(timeout, fut).await.map_err(|_| {
        Error::Conn(format!(
            "connect {addr}: {what} timed out after {timeout:?}"
        ))
    })
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
