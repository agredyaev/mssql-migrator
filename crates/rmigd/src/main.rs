//! `rmigd` — session daemon for warm SQL connection reuse.
//!
//! ### Purpose
//! Listens on a Unix socket, accepts session tokens, and keeps one warm SQL
//! Server connection alive across multiple `rmig` invocations. Avoids the
//! per-invocation connection overhead of TDS login + catalog warm-up.
//!
//! ### Environment
//! - `RMIGD_ENV` — dotenv path (optional; if absent, uses `.env` non-required)
//! - Socket path resolved via [`migrator_core::session::resolve_socket_path`]
//!
//! ### Nominal flow
//! 1. Resolve socket path.
//! 2. Load environment if `RMIGD_ENV` is set.
//! 3. Enter event loop: accept connections, delegate to `run_daemon`.
//! 4. Bound concurrent socket handlers to `MAX_DAEMON_CLIENTS`; SQL calls still
//!    serialize through one warm TDS session.
//!
//! ### Off-nominal
//! - Socket file collision: daemon fails with address-in-use.
//! - `RMIGD_ENV` points to missing file: daemon starts but all sessions use ambient env.

use std::path::PathBuf;
use std::process::ExitCode;

use tracing_subscriber::EnvFilter;

#[tokio::main]
async fn main() -> ExitCode {
    init_tracing();
    match run().await {
        Ok(()) => ExitCode::from(migrator_core::error::EXIT_OK as u8),
        Err(code) => ExitCode::from(code as u8),
    }
}

/// Resolve the socket, load env, and serve. Errors print via `Display` (not the
/// `Termination` `Debug` dump) and map to the documented exit-code scheme
/// (`crates/core/src/error.rs`) so CI can classify daemon startup failures:
/// socket/env resolution keeps its classified code; an opaque serve failure is
/// `EXIT_GENERAL`.
async fn run() -> Result<(), i32> {
    let socket = migrator_core::session::resolve_socket_path().map_err(|e| {
        eprintln!("rmigd: {e}");
        e.exit_code()
    })?;
    let env_required = std::env::var("RMIGD_ENV").is_ok();
    let env = std::env::var("RMIGD_ENV").unwrap_or_else(|_| ".env".into());
    migrator_core::session::run_daemon(&socket, PathBuf::from(env).as_path(), env_required)
        .await
        .map_err(|e| {
            eprintln!("rmigd: {e:#}");
            migrator_core::error::EXIT_GENERAL
        })
}

fn init_tracing() {
    let default_level = std::env::var("RM_LOG_LEVEL").unwrap_or_else(|_| "info".into());
    let filter = EnvFilter::try_from_default_env()
        .or_else(|_| EnvFilter::try_new(default_log_filter(&default_level)))
        .unwrap_or_else(|_| EnvFilter::new(default_log_filter("info")));
    let _ = tracing_subscriber::fmt()
        .with_env_filter(filter)
        .with_writer(std::io::stderr)
        .try_init();
}

fn default_log_filter(log_level: &str) -> String {
    let level = if log_level.trim().is_empty() {
        "info"
    } else {
        log_level.trim()
    };
    if level.contains('=') || level.contains(',') {
        level.to_string()
    } else {
        format!("warn,migrator_core={level},rmig={level},rmigd={level}")
    }
}

#[cfg(test)]
mod tests {
    use super::default_log_filter;

    #[test]
    fn default_log_filter_scopes_simple_level_to_project_crates() {
        assert_eq!(
            default_log_filter("debug"),
            "warn,migrator_core=debug,rmig=debug,rmigd=debug"
        );
    }

    #[test]
    fn default_log_filter_preserves_explicit_env_filter() {
        assert_eq!(
            default_log_filter("migrator_core=trace,tiberius=warn"),
            "migrator_core=trace,tiberius=warn"
        );
    }
}
