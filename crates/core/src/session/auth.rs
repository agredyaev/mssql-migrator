//! rmigd connection authorization (shared token + private socket path).

#[cfg(feature = "session-daemon")]
use crate::error::{Error, Result};

/// Returns true when daemon must receive `Request::Auth` before SQL RPC.
#[cfg(feature = "session-daemon")]
pub fn token_required() -> bool {
    !session_token_from_env().is_empty()
}

pub fn session_token_from_env() -> String {
    std::env::var("RMIG_SESSION_TOKEN").unwrap_or_default()
}

#[cfg(feature = "session-daemon")]
pub fn verify_token(provided: &str) -> Result<()> {
    let expected = session_token_from_env();
    if expected.is_empty() {
        return Ok(());
    }
    if provided == expected {
        Ok(())
    } else {
        Err(Error::InvalidInput("rmigd: invalid session token".into()))
    }
}
