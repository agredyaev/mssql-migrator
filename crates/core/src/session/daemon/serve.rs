use std::sync::Arc;
use std::time::Duration;

use tokio::io::BufReader;

use crate::driver::RawClient;
use crate::error::Result;

use super::super::daemon_rpc;

/// Serve one client socket. The shared TDS session is cleaned on EVERY exit
/// path — including mid-response write failures — so a dying client can never
/// leave an open transaction or a held advisory lock to the next socket.
pub async fn serve(
    stream: tokio::net::UnixStream,
    client: Arc<tokio::sync::Mutex<RawClient>>,
    command_timeout: Duration,
    endpoint: Arc<super::endpoint::Endpoint>,
) -> Result<()> {
    let (read_half, mut write_half) = stream.into_split();
    let mut reader = BufReader::new(read_half);
    let mut session: Option<tokio::sync::OwnedMutexGuard<RawClient>> = None;
    let result = super::serve_loop::serve_loop(
        &mut reader,
        &mut write_half,
        &client,
        command_timeout,
        &endpoint,
        &mut session,
    )
    .await;
    if let Some(mut guard) = session {
        daemon_rpc::cleanup_session(&mut guard).await;
    }
    result
}
