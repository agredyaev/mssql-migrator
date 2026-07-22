//! Unix socket daemon holding one warm TDS connection.
//!
//! ### Purpose
//! Listens on a Unix socket, accepts session requests, and multiplexes them
//! over a single pre-warmed TDS connection. Avoids per-invocation TDS login
//! overhead and catalog warm-up for repeated `rmig` operations.
//!
//! ### Non-obvious behaviour
//! - **Single connection, concurrent clients**: All spawned tasks share one
//!   mutex-protected optional TDS client. Requests are serialised; a client
//!   whose command times out is discarded and reconnected before reuse.
//! - **Async mutex**: Uses `tokio::sync::Mutex` (not `std::sync::Mutex`)
//!   because the lock is held across `.await` points in RPC handlers.
//! - **Feature-gated**: Entire module is `#[cfg(feature = "session-daemon")]`.
//!   Without the feature, `run_daemon`, `verify_token`, etc. are unavailable.
//! - **Client backpressure**: At most `MAX_DAEMON_CLIENTS` socket handlers run
//!   at once. Extra accepted sockets wait for a handler slot instead of
//!   spawning unbounded tasks.
//! - **Socket lifecycle**: Existing socket file is removed on startup.
//!   Directory parents are created if missing.

use std::path::Path;
use std::sync::Arc;

use tokio::net::UnixListener;
use tokio::sync::{Mutex, Semaphore};

use crate::config::Config;
use crate::driver::connect;
use crate::session::limits::MAX_DAEMON_CLIENTS;

mod endpoint;
mod reply;
mod serve;
mod serve_loop;
use super::socket::restrict_socket_mode;
use serve::serve;

/// Starts the rmigd Unix-socket daemon, accepting connections until the process exits.
pub async fn run_daemon(socket: &Path, cfg: Config) -> anyhow::Result<()> {
    super::auth::apply_session_token_from_config(&cfg);
    let conn = connect(&cfg).await?;
    let shared = Arc::new(Mutex::new(Some(conn.client)));
    let reconnect_cfg = Arc::new(cfg.clone());
    let socket = if !cfg.session_socket.is_empty() {
        std::path::PathBuf::from(&cfg.session_socket)
    } else {
        socket.to_path_buf()
    };
    if socket.exists() {
        // If a live daemon is already listening, refuse to start rather than
        // silently removing its socket and orphaning it (with its warm TDS
        // session and any held advisory lock). Only a stale socket is removed.
        if tokio::net::UnixStream::connect(&socket).await.is_ok() {
            anyhow::bail!(
                "rmigd: a daemon is already listening on {}",
                socket.display()
            );
        }
        std::fs::remove_file(&socket)?;
    }
    if let Some(parent) = socket.parent() {
        if !parent.as_os_str().is_empty() {
            // Same contract as resolve_socket_path: create privately, or
            // require an existing parent to already be private — never chmod
            // a caller-owned directory.
            super::socket::ensure_private_parent(parent)?;
        }
    }
    let listener = UnixListener::bind(&socket)?;
    restrict_socket_mode(&socket)?;
    tracing::info!(socket = %socket.display(), "rmigd listening");
    let client_slots = Arc::new(Semaphore::new(MAX_DAEMON_CLIENTS));
    let command_timeout = cfg.command_timeout;
    let daemon_endpoint = Arc::new(endpoint::Endpoint {
        server: cfg.server.clone(),
        port: cfg.port.clone(),
        user: cfg.user.clone(),
        encrypt: cfg.encrypt,
        trust_server_certificate: cfg.trust_server_certificate,
    });
    loop {
        // A transient accept error (e.g. EMFILE under fd pressure) must not kill
        // the daemon; log, back off briefly, and keep serving.
        let (stream, _) = match listener.accept().await {
            Ok(pair) => pair,
            Err(e) => {
                tracing::warn!(error = %e, "rmigd accept failed; continuing");
                tokio::time::sleep(std::time::Duration::from_millis(50)).await;
                continue;
            }
        };
        let permit = client_slots.clone().acquire_owned().await?;
        let client = shared.clone();
        let cfg = reconnect_cfg.clone();
        let ep = daemon_endpoint.clone();
        tokio::spawn(async move {
            let _permit = permit;
            if let Err(e) = serve(stream, client, cfg, command_timeout, ep).await {
                tracing::warn!(error = %e, "rmigd client failed");
            }
        });
    }
}
