//! Unix socket daemon holding one warm TDS connection.

use std::path::Path;
use std::sync::Arc;

use tokio::io::{AsyncBufReadExt, AsyncWriteExt, BufReader};
use tokio::net::UnixListener;
use tokio::sync::Mutex;

use crate::config::{build_config, load_env_file, validate_config};
use crate::driver::{connect, RawClient};
use crate::error::{Error, Result};

use super::auth::{token_required, verify_token};
use super::daemon_rpc;
use super::limits::MAX_SESSION_LINE_BYTES;
use super::protocol::{Request, Response};
use super::socket::{resolve_socket_path, restrict_socket_mode};

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

async fn serve(stream: tokio::net::UnixStream, client: Arc<Mutex<RawClient>>) -> Result<()> {
    let (read_half, mut write_half) = stream.into_split();
    let mut reader = BufReader::new(read_half);
    let need_token = token_required();
    let mut authenticated = !need_token;

    loop {
        let mut line = String::new();
        let n = reader.read_line(&mut line).await.map_err(Error::Io)?;
        if n == 0 {
            break;
        }
        if line.len() > MAX_SESSION_LINE_BYTES {
            write_response(
                &mut write_half,
                Response::err("request line exceeds size limit"),
            )
            .await?;
            break;
        }
        let req: Request = match serde_json::from_str(line.trim()) {
            Ok(r) => r,
            Err(e) => {
                write_response(&mut write_half, Response::err(e.to_string())).await?;
                continue;
            }
        };

        if matches!(&req, Request::Auth { .. }) {
            if let Request::Auth { token } = req {
                match verify_token(&token) {
                    Ok(()) => {
                        authenticated = true;
                        write_response(&mut write_half, Response::ok_empty()).await?;
                    }
                    Err(e) => {
                        write_response(&mut write_half, Response::err(e.to_string())).await?;
                        break;
                    }
                }
            }
            continue;
        }

        if !authenticated {
            write_response(
                &mut write_half,
                Response::err("rmigd: auth required (send Auth with RMIG_SESSION_TOKEN)"),
            )
            .await?;
            break;
        }

        let resp = daemon_rpc::handle(&client, req).await;
        write_response(&mut write_half, resp).await?;
    }
    Ok(())
}

async fn write_response(
    write_half: &mut tokio::net::unix::OwnedWriteHalf,
    resp: Response,
) -> Result<()> {
    let mut out = serde_json::to_string(&resp).map_err(|e| Error::Other(e.into()))?;
    if out.len() > MAX_SESSION_LINE_BYTES {
        return Err(Error::InvalidInput("response exceeds size limit".into()));
    }
    out.push('\n');
    write_half
        .write_all(out.as_bytes())
        .await
        .map_err(Error::Io)?;
    Ok(())
}
