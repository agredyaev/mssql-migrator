use tokio::io::{AsyncBufReadExt, AsyncReadExt, AsyncWriteExt, BufReader};
use tokio::net::unix::OwnedReadHalf;
use tokio::net::{unix::OwnedWriteHalf, UnixStream};

use crate::driver::RowData;
use crate::error::{Error, Result};

use super::auth::resolve_session_token;
use super::limits::MAX_SESSION_LINE_BYTES;
use super::protocol::{Request, Response};
use crate::config::Config;

/// Client for communicating with the `rmigd` daemon over a Unix-domain socket.
pub struct ProxyClient {
    reader: BufReader<OwnedReadHalf>,
    writer: OwnedWriteHalf,
}

impl ProxyClient {
    /// Connects to the daemon at `socket_path`, performing auth and a ping handshake.
    pub async fn connect(socket_path: &str, cfg: Option<&Config>) -> Result<Self> {
        // Bound the whole connect (socket + auth + ping) so a wedged daemon causes
        // a fallback to direct SQL (see `session::client`) instead of hanging CI.
        match cfg.map(|c| c.command_timeout).filter(|d| !d.is_zero()) {
            Some(t) => tokio::time::timeout(t, Self::connect_inner(socket_path, cfg))
                .await
                .map_err(|_| {
                    Error::Config(format!(
                        "rmigd connect {socket_path}: timed out after {t:?}"
                    ))
                })?,
            None => Self::connect_inner(socket_path, cfg).await,
        }
    }

    async fn connect_inner(socket_path: &str, cfg: Option<&Config>) -> Result<Self> {
        let stream = UnixStream::connect(socket_path)
            .await
            .map_err(|e| Error::Config(format!("rmigd connect {}: {e}", socket_path)))?;
        let (read_half, write_half) = stream.into_split();
        let mut client = Self {
            reader: BufReader::new(read_half),
            writer: write_half,
        };
        let token = resolve_session_token(cfg);
        if !token.is_empty() {
            client
                .call(Request::Auth {
                    token: token.clone(),
                })
                .await?
                .into_result()
                .map(|_| ())?;
        }
        // Send the configured SQL endpoint with the handshake: the daemon holds
        // one warm connection from ITS environment, and a session must not
        // silently execute against a different server than this CLI intends.
        let (server, port, user) = cfg
            .map(|c| (c.server.clone(), c.port.clone(), c.user.clone()))
            .unwrap_or_default();
        client
            .call(Request::Ping { server, port, user })
            .await?
            .into_result()
            .map(|_| ())?;
        Ok(client)
    }

    /// Sends a SQL exec request to the daemon and waits for acknowledgement.
    pub async fn exec(&mut self, sql: &str) -> Result<()> {
        self.call(Request::Exec {
            sql: sql.to_string(),
        })
        .await?
        .into_result()
        .map(|_| ())
    }

    /// Sends a SQL query request to the daemon and returns the resulting rows.
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
            return Err(Error::InvalidInput(
                "rmigd request exceeds size limit".into(),
            ));
        }
        line.push('\n');
        self.writer
            .write_all(line.as_bytes())
            .await
            .map_err(|e| Error::Other(e.into()))?;
        let mut resp_line = String::new();
        // Cap the read itself (limit + 1 detects overflow): checking length
        // only AFTER read_line lets a hostile/wedged endpoint grow the buffer
        // without bound before the check runs.
        let mut limited = (&mut self.reader).take(MAX_SESSION_LINE_BYTES as u64 + 1);
        limited
            .read_line(&mut resp_line)
            .await
            .map_err(|e| Error::Other(e.into()))?;
        if resp_line.len() > MAX_SESSION_LINE_BYTES {
            return Err(Error::InvalidInput(
                "rmigd response exceeds size limit".into(),
            ));
        }
        serde_json::from_str(&resp_line).map_err(|e| Error::Other(e.into()))
    }
}

#[cfg(test)]
#[path = "../tests/proxy_test.rs"]
mod proxy_tests;
