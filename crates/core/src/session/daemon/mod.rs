//! Unix socket daemon holding one warm TDS connection.
//!
//! ### Purpose
//! Listens on a Unix socket, accepts session requests, and multiplexes them
//! over a single pre-warmed TDS connection. Avoids per-invocation TDS login
//! overhead and catalog warm-up for repeated `rmig` operations.
//!
//! ### Non-obvious behaviour
//! - **Single connection, concurrent clients**: All spawned tasks share one
//!   `Arc<tokio::sync::Mutex<RawClient>>`. Requests are serialised at the
//!   TDS level — each waits for the previous query to finish.
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

use crate::config::{build_config, load_env_file, load_env_file_required, validate_config};
use crate::driver::connect;
use crate::session::limits::MAX_DAEMON_CLIENTS;

mod endpoint;
mod reply;
mod serve;
use super::socket::{resolve_socket_path, restrict_dir_mode, restrict_socket_mode};
use serve::serve;

/// Starts the rmigd Unix-socket daemon, accepting connections until the process exits.
pub async fn run_daemon(socket: &Path, env_path: &Path, env_required: bool) -> anyhow::Result<()> {
    let env = if env_required {
        load_env_file_required(env_path)?
    } else {
        load_env_file(env_path)?
    };
    let mut cfg = build_config(&env, false);
    validate_config(&mut cfg).map_err(|e| anyhow::anyhow!("{e}"))?;
    super::auth::apply_session_token_from_config(&cfg);
    if super::auth::resolve_session_token(Some(&cfg)).is_empty() {
        tracing::warn!(
            "rmigd running WITHOUT a session token: socket file permissions are the only access control"
        );
    }
    let conn = connect(&cfg).await?;
    let shared = Arc::new(Mutex::new(conn.client));
    let socket = if socket.as_os_str().is_empty() {
        resolve_socket_path()?
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
            std::fs::create_dir_all(parent)?;
            // Harden the parent for explicit socket paths too (resolve_socket_path
            // does this for the default path); a group/world-traversable parent
            // would expose the socket regardless of its own 0600 mode.
            restrict_dir_mode(parent)?;
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
        let ep = daemon_endpoint.clone();
        tokio::spawn(async move {
            let _permit = permit;
            if let Err(e) = serve(stream, client, command_timeout, ep).await {
                tracing::warn!(error = %e, "rmigd client failed");
            }
        });
    }
}
