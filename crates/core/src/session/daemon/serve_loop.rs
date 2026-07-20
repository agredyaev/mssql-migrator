use std::sync::Arc;
use std::time::Duration;

use tokio::io::{AsyncBufReadExt, AsyncReadExt, BufReader};
use tokio::net::unix::{OwnedReadHalf, OwnedWriteHalf};

use crate::config::Config;
use crate::driver::RawClient;
use crate::error::{Error, Result};

use super::super::auth::{token_required, verify_token};
use super::super::limits::MAX_SESSION_LINE_BYTES;
use super::super::protocol::{Request, Response};
use super::reply::write_response;

/// Drop a connection that sends nothing for this long, so an idle (or slowloris)
/// client cannot hold one of the bounded handler slots indefinitely.
const IDLE_TIMEOUT: Duration = Duration::from_secs(60);

pub(super) async fn serve_loop(
    reader: &mut BufReader<OwnedReadHalf>,
    write_half: &mut OwnedWriteHalf,
    client: &Arc<tokio::sync::Mutex<Option<RawClient>>>,
    reconnect_cfg: &Config,
    command_timeout: Duration,
    endpoint: &super::endpoint::Endpoint,
    session: &mut Option<tokio::sync::OwnedMutexGuard<Option<RawClient>>>,
) -> Result<()> {
    let need_token = token_required();
    let mut authenticated = !need_token;

    loop {
        let mut line = String::new();
        // Bound the read to the size cap + 1 so a client streaming a huge line
        // without a newline cannot force unbounded buffering before the check
        // below; wrap in an idle timeout so a silent client frees its slot.
        let mut limited = (&mut *reader).take(MAX_SESSION_LINE_BYTES as u64 + 1);
        let n = match tokio::time::timeout(IDLE_TIMEOUT, limited.read_line(&mut line)).await {
            Ok(r) => r.map_err(Error::Io)?,
            Err(_) => break,
        };
        if n == 0 {
            break;
        }
        if line.len() > MAX_SESSION_LINE_BYTES {
            write_response(write_half, Response::err("request line exceeds size limit")).await?;
            break;
        }
        let req: Request = match serde_json::from_str(line.trim()) {
            Ok(r) => r,
            Err(e) => {
                write_response(write_half, Response::err(e.to_string())).await?;
                continue;
            }
        };

        if matches!(&req, Request::Auth { .. }) {
            if let Request::Auth { token } = req {
                match verify_token(&token) {
                    Ok(()) => {
                        authenticated = true;
                        write_response(write_half, Response::ok_empty()).await?;
                    }
                    Err(e) => {
                        write_response(write_half, Response::err(e.to_string())).await?;
                        break;
                    }
                }
            }
            continue;
        }

        if !authenticated {
            write_response(
                write_half,
                Response::err("rmigd: auth required (send Auth with RMIG_SESSION_TOKEN)"),
            )
            .await?;
            break;
        }

        // Daemon-level metrics/health pull: no warm session or SQL Server needed.
        if matches!(&req, Request::Stats {}) {
            let warm = client.try_lock().map(|g| g.is_some()).unwrap_or(true);
            let resp = Response::stats(super::metrics::snapshot_json(warm));
            write_response(write_half, resp).await?;
            continue;
        }

        // Refuse a session whose declared SQL endpoint differs from the warm
        // connection's: the CLI would otherwise plan and apply against the
        // daemon's server while reporting its own configured target.
        if let Some(resp) = super::endpoint::refusal_for(&req, endpoint) {
            write_response(write_half, resp).await?;
            break;
        }

        if !super::serve::acquire_session(session, client, reconnect_cfg, write_half).await? {
            break;
        }
        let Some(guard) = session.as_deref_mut() else {
            break;
        };
        super::metrics::record_request();
        match super::serve::dispatch(guard, req, command_timeout).await? {
            Some(resp) => write_response(write_half, resp).await?,
            None => {
                write_response(write_half, Response::err("rmigd: request timed out")).await?;
                break;
            }
        }
    }
    Ok(())
}
