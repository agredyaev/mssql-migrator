//! rmigd connection authorization (shared token + private socket path).

use crate::config::Config;

#[cfg(feature = "session-daemon")]
use crate::error::{Error, Result};

/// Resolve the session token from config (dotenv) or process environment.
pub fn resolve_session_token(cfg: Option<&Config>) -> String {
    if let Some(cfg) = cfg {
        if !cfg.session_token.is_empty() {
            return cfg.session_token.clone();
        }
    }
    std::env::var("RMIG_SESSION_TOKEN").unwrap_or_default()
}

/// Publish a dotenv-loaded token for daemon-side auth checks in this process.
pub fn apply_session_token_from_config(cfg: &Config) {
    if !cfg.session_token.is_empty() {
        std::env::set_var("RMIG_SESSION_TOKEN", &cfg.session_token);
    }
}

/// Returns true when daemon must receive `Request::Auth` before SQL RPC.
#[cfg(feature = "session-daemon")]
pub fn token_required() -> bool {
    !resolve_session_token(None).is_empty()
}

/// Returns the session token from the process environment.
pub fn session_token_from_env() -> String {
    resolve_session_token(None)
}

/// Constant-time byte comparison so token verification does not leak how many
/// leading bytes matched via response timing. The length difference is allowed to
/// short-circuit (token length is not sensitive).
#[cfg(feature = "session-daemon")]
fn constant_time_eq(a: &[u8], b: &[u8]) -> bool {
    if a.len() != b.len() {
        return false;
    }
    let mut diff = 0u8;
    for (x, y) in a.iter().zip(b.iter()) {
        diff |= x ^ y;
    }
    diff == 0
}

/// Returns `Ok` when `provided` matches the configured session token, or errors otherwise.
#[cfg(feature = "session-daemon")]
pub fn verify_token(provided: &str) -> Result<()> {
    let expected = resolve_session_token(None);
    if expected.is_empty() {
        return Ok(());
    }
    if constant_time_eq(provided.as_bytes(), expected.as_bytes()) {
        Ok(())
    } else {
        Err(Error::InvalidInput("rmigd: invalid session token".into()))
    }
}

#[cfg(test)]
#[path = "../tests/auth_test.rs"]
mod tests;
