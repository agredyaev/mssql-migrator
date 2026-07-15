use tokio::io::AsyncWriteExt;

use crate::error::{Error, Result};

use super::super::limits::MAX_SESSION_LINE_BYTES;
use super::super::protocol::Response;

/// Serialize `resp` as one newline-terminated JSON line to the client socket.
pub(super) async fn write_response(
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
