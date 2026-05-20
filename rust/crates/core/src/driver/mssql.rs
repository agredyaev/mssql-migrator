use tiberius::{Client, Config as TdsConfig};
use tokio::net::TcpStream;
use tokio_util::compat::{Compat, TokioAsyncWriteCompatExt};

use crate::config::Config;
use crate::error::{Error, Result};

pub type RawClient = Client<Compat<TcpStream>>;

pub struct MssqlConn {
    pub client: RawClient,
}

pub async fn connect(cfg: &Config) -> Result<MssqlConn> {
    let mut tds = TdsConfig::new();
    tds.host(&cfg.server);
    if let Ok(port) = cfg.port.parse::<u16>() {
        tds.port(port);
    }
    tds.database(&cfg.database);
    tds.authentication(tiberius::AuthMethod::sql_server(&cfg.user, &cfg.password));
    if cfg.trust_server_certificate {
        tds.trust_cert();
    }
    if cfg.encrypt {
        tds.encryption(tiberius::EncryptionLevel::Required);
    } else {
        tds.encryption(tiberius::EncryptionLevel::NotSupported);
    }
    let addr = format!("{}:{}", cfg.server, cfg.port);
    let tcp = TcpStream::connect(&addr)
        .await
        .map_err(|e| Error::Sql(format!("connect {addr}: {e}")))?;
    tcp.set_nodelay(true).ok();
    let mut client = Client::connect(tds, tcp.compat_write())
        .await
        .map_err(|e| Error::Sql(format!("tds handshake: {e}")))?;
    init_session(&mut client).await?;
    Ok(MssqlConn { client })
}

async fn init_session(client: &mut RawClient) -> Result<()> {
    exec(
        client,
        "SET QUOTED_IDENTIFIER ON; SET ANSI_NULLS ON; SET ANSI_PADDING ON;",
    )
    .await
}

pub async fn ping(client: &mut RawClient) -> Result<()> {
    client
        .simple_query("SELECT 1")
        .await
        .map_err(|e| Error::Sql(e.to_string()))?;
    Ok(())
}

pub async fn exec(client: &mut RawClient, sql: &str) -> Result<()> {
    client
        .simple_query(sql)
        .await
        .map_err(|e| Error::Sql(e.to_string()))?
        .into_results()
        .await
        .map_err(|e| Error::Sql(e.to_string()))?;
    Ok(())
}

pub async fn query_tiberius(
    client: &mut RawClient,
    sql: &str,
    params: &[&dyn tiberius::ToSql],
) -> Result<Vec<tiberius::Row>> {
    client
        .query(sql, params)
        .await
        .map_err(|e| Error::Sql(e.to_string()))?
        .into_first_result()
        .await
        .map_err(|e| Error::Sql(e.to_string()))
}

/// All result sets from one batch (single round-trip).
pub async fn query_all_results(
    client: &mut RawClient,
    sql: &str,
    params: &[&dyn tiberius::ToSql],
) -> Result<Vec<Vec<tiberius::Row>>> {
    client
        .query(sql, params)
        .await
        .map_err(|e| Error::Sql(e.to_string()))?
        .into_results()
        .await
        .map_err(|e| Error::Sql(e.to_string()))
}
