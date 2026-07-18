//! `rmigd` — session daemon for warm SQL connection reuse.
//!
//! ### Purpose
//! Listens on a Unix socket, accepts session tokens, and keeps one warm SQL
//! Server connection alive across multiple `rmig` invocations. Avoids the
//! per-invocation connection overhead of TDS login + catalog warm-up.
//!
//! ### Configuration
//! - `RMIGD_CONFIG` — typed TOML path (default `config.toml`)
//! - Socket path resolved via [`migrator_core::session::resolve_socket_path`]
//!
//! ### Nominal flow
//! 1. Resolve socket path.
//! 2. Load TOML and environment-only credentials.
//! 3. Enter event loop: accept connections, delegate to `run_daemon`.
//! 4. Bound concurrent socket handlers to `MAX_DAEMON_CLIENTS`; SQL calls still
//!    serialize through one warm TDS session.
//!
//! ### Off-nominal
//! - Socket file collision: if a live daemon answers on the socket, startup is
//!   refused; a stale socket file is removed and replaced.
//! - A missing config file or environment-only credential fails startup.

#![forbid(unsafe_code)]
use std::process::ExitCode;

use tracing_subscriber::EnvFilter;

#[tokio::main]
async fn main() -> ExitCode {
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
    let file = match std::env::var("RMIGD_CONFIG") {
        Ok(path) => migrator_core::config::load_toml_config_required(std::path::Path::new(&path)),
        Err(_) => migrator_core::config::load_toml_config(std::path::Path::new("config.toml")),
    }
    .map_err(report)?;
    let mut cfg = migrator_core::config::build_config(&file, false);
    migrator_core::config::validate_daemon_config(&mut cfg).map_err(report)?;
    init_tracing(&cfg.log_level);
    let socket = migrator_core::session::resolve_socket_path().map_err(|e| {
        eprintln!("rmigd: {e}");
        e.exit_code()
    })?;
    migrator_core::session::run_daemon(&socket, cfg)
        .await
        .map_err(|e| {
            eprintln!("rmigd: {e:#}");
            migrator_core::error::EXIT_GENERAL
        })
}

fn report(e: migrator_core::Error) -> i32 {
    eprintln!("rmigd: {e}");
    e.exit_code()
}

fn init_tracing(default_level: &str) {
    let filter = EnvFilter::try_from_default_env()
        .or_else(|_| EnvFilter::try_new(default_log_filter(default_level)))
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
