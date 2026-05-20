use tokio::io::{AsyncBufReadExt, AsyncWriteExt, BufReader};
use tokio::net::unix::OwnedReadHalf;
use tokio::net::{unix::OwnedWriteHalf, UnixStream};

use crate::driver::RowData;
use crate::error::{Error, Result};

use super::auth::session_token_from_env;
use super::limits::MAX_SESSION_LINE_BYTES;
use super::protocol::{Request, Response};

pub struct ProxyClient {
    reader: BufReader<OwnedReadHalf>,
    writer: OwnedWriteHalf,
}

impl ProxyClient {
    pub async fn connect(socket_path: &str) -> Result<Self> {
        let stream = UnixStream::connect(socket_path)
            .await
            .map_err(|e| Error::Config(format!("rmigd connect {}: {e}", socket_path)))?;
        let (read_half, write_half) = stream.into_split();
        let mut client = Self {
            reader: BufReader::new(read_half),
            writer: write_half,
        };
        let token = session_token_from_env();
        if !token.is_empty() {
            client
                .call(Request::Auth {
                    token: token.clone(),
                })
                .await?
                .into_result()
                .map(|_| ())?;
        }
        client
            .call(Request::Ping)
            .await?
            .into_result()
            .map(|_| ())?;
        Ok(client)
    }

    pub async fn exec(&mut self, sql: &str) -> Result<()> {
        self.call(Request::Exec {
            sql: sql.to_string(),
        })
        .await?
        .into_result()
        .map(|_| ())
    }

    pub async fn query(&mut self, sql: &str, params: &[&str]) -> Result<Vec<RowData>> {
        self.call(Request::Query {
            sql: sql.to_string(),
            params: params.iter().map(|s| (*s).to_string()).collect(),
        })
        .await?
        .into_result()
    }

    async fn call(&mut self, req: Request) -> Result<Response> {
        let mut line = serde_json::to_string(&req).map_err(|e| Error::Other(e.into()))?;
        if line.len() > MAX_SESSION_LINE_BYTES {
            return Err(Error::InvalidInput("rmigd request exceeds size limit".into()));
        }
        line.push('\n');
        self.writer
            .write_all(line.as_bytes())
            .await
            .map_err(|e| Error::Other(e.into()))?;
        let mut resp_line = String::new();
        self.reader
            .read_line(&mut resp_line)
            .await
            .map_err(|e| Error::Other(e.into()))?;
        if resp_line.len() > MAX_SESSION_LINE_BYTES {
            return Err(Error::InvalidInput("rmigd response exceeds size limit".into()));
        }
        serde_json::from_str(&resp_line).map_err(|e| Error::Other(e.into()))
    }
}
