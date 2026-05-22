//! Unix socket daemon holding one warm TDS connection.

use std::path::Path;
use std::sync::Arc;

use tokio::net::UnixListener;
use tokio::sync::Mutex;

use crate::config::{build_config, load_env_file, validate_config};
use crate::driver::connect;

mod serve;
use super::socket::{resolve_socket_path, restrict_socket_mode};
use serve::serve;

pub async fn run_daemon(socket: &Path, env_path: &Path) -> anyhow::Result<()> {
    let env = load_env_file(env_path)?;
    let mut cfg = build_config(&env, false);
    validate_config(&mut cfg).map_err(|e| anyhow::anyhow!("{e}"))?;
    let conn = connect(&cfg).await?;
    let shared = Arc::new(Mutex::new(conn.client));
    let socket = if socket.as_os_str().is_empty() {
        resolve_socket_path()?
    } else {
        socket.to_path_buf()
    };
    if socket.exists() {
        std::fs::remove_file(&socket)?;
    }
    if let Some(parent) = socket.parent() {
        if !parent.as_os_str().is_empty() {
            std::fs::create_dir_all(parent)?;
        }
    }
    let listener = UnixListener::bind(&socket)?;
    restrict_socket_mode(&socket)?;
    eprintln!("rmigd listening on {}", socket.display());
    loop {
        let (stream, _) = listener.accept().await?;
        let client = shared.clone();
        tokio::spawn(async move {
            if let Err(e) = serve(stream, client).await {
                eprintln!("rmigd client: {e}");
            }
        });
    }
}
