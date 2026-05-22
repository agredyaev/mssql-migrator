use std::sync::Arc;

use tokio::io::{AsyncBufReadExt, AsyncWriteExt, BufReader};

use crate::driver::RawClient;
use crate::error::{Error, Result};

use super::super::auth::{token_required, verify_token};
use super::super::daemon_rpc;
use super::super::limits::MAX_SESSION_LINE_BYTES;
use super::super::protocol::{Request, Response};

pub async fn serve(
    stream: tokio::net::UnixStream,
    client: Arc<tokio::sync::Mutex<RawClient>>,
) -> Result<()> {
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
