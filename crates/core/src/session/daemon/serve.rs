use std::sync::Arc;
use std::time::Duration;

use tokio::io::BufReader;

use crate::config::Config;
use crate::driver::RawClient;
use crate::error::{Error, Result};

use super::super::daemon_rpc;
use super::super::protocol::{Request, Response};

pub(super) async fn reconnect(client: &mut Option<RawClient>, cfg: &Config) -> Result<()> {
    if client.is_none() {
        *client = Some(crate::driver::connect(cfg).await?.client);
    }
    Ok(())
}

pub(super) async fn dispatch(
    session: &mut Option<RawClient>,
    req: Request,
    timeout: Duration,
) -> Result<Option<Response>> {
    let Some(client) = session.as_mut() else {
        return Err(Error::Config("rmigd: missing database session".into()));
    };
    if timeout.is_zero() {
        return Ok(Some(daemon_rpc::handle(client, req).await));
    }
    match tokio::time::timeout(timeout, daemon_rpc::handle(client, req)).await {
        Ok(response) => Ok(Some(response)),
        Err(_) => {
            // A dropped Tiberius future can leave unread packets and an invalid
            // transaction descriptor. Discard the TCP session before reuse.
            session.take();
            Ok(None)
        }
    }
}

/// Serve one client socket. A healthy shared TDS session is cleaned on every
/// exit path. A timed-out session is discarded because a cancelled Tiberius
/// future is not safe to reuse; SQL Server rolls it back on disconnect.
pub async fn serve(
    stream: tokio::net::UnixStream,
    client: Arc<tokio::sync::Mutex<Option<RawClient>>>,
    reconnect_cfg: Arc<Config>,
    command_timeout: Duration,
    endpoint: Arc<super::endpoint::Endpoint>,
) -> Result<()> {
    let (read_half, mut write_half) = stream.into_split();
    let mut reader = BufReader::new(read_half);
    let mut session: Option<tokio::sync::OwnedMutexGuard<Option<RawClient>>> = None;
    let result = super::serve_loop::serve_loop(
        &mut reader,
        &mut write_half,
        &client,
        &reconnect_cfg,
        command_timeout,
        &endpoint,
        &mut session,
    )
    .await;
    if let Some(mut guard) = session {
        let cleanup = match guard.as_mut() {
            Some(client) => daemon_rpc::cleanup_session(client).await,
            None => Ok(()),
        };
        if let Err(e) = cleanup {
            tracing::warn!(error = %e, "rmigd session cleanup failed; discarding connection");
            guard.take();
        }
    }
    result
}
