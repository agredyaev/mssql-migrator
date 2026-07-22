use crate::error::{Error, Result};

use super::catalog::discover_catalog_databases;
use super::env_parse::validate_boolean_envs;
use super::Config;

/// Validates required fields in `cfg`, returning an error if any mandatory env vars are absent.
pub fn validate_config(cfg: &mut Config) -> Result<()> {
    validate_boolean_envs()?;
    let mut missing = Vec::new();
    let mut missing_secrets = Vec::new();
    if cfg.server.is_empty() {
        missing.push("RM_DB_SERVER");
    }
    if cfg.sql_root.is_empty() {
        missing.push("RM_SQL_ROOT");
    }
    if cfg.user.is_empty() {
        missing_secrets.push("RM_DB_USER");
    }
    if cfg.password.is_empty() {
        missing_secrets.push("RM_DB_PASSWORD");
    }
    if !missing_secrets.is_empty() {
        return Err(Error::Config(format!(
            "missing required process environment variable(s): {}; SQL credentials are not read from config.toml",
            missing_secrets.join(", ")
        )));
    }
    if !cfg.session_socket.is_empty() && cfg.session_token.is_empty() {
        return Err(Error::Config(
            "RMIG_SESSION_TOKEN is required in the process environment when using rmigd; the token is not read from config.toml".into(),
        ));
    }
    if !missing.is_empty() {
        return Err(Error::Config(format!(
            "missing required variables: {}",
            missing.join(", ")
        )));
    }
    // Reject control characters (NUL, newlines, etc.) in connection identity so they
    // cannot corrupt diagnostics/logs. Backslashes are allowed: SQL Server named
    // instances use `host\INSTANCE`.
    reject_control_chars("RM_DB_SERVER", &cfg.server)?;
    reject_control_chars("database name", &cfg.database)?;
    // Empty means "use the driver default"; a non-empty value must be a real
    // port. Zero parses as u16 but is not a reachable TCP destination.
    if !cfg.port.is_empty() && !matches!(cfg.port.parse::<u16>(), Ok(1..)) {
        return Err(Error::Config(format!(
            "RM_DB_PORT is not a valid TCP port (1-65535): {}",
            cfg.port
        )));
    }
    normalize_catalog_paths(cfg)?;
    Ok(())
}

/// Daemons always expose a token-authenticated transport, even when their
/// socket path uses the platform default rather than `RMIG_SESSION`.
pub fn validate_daemon_config(cfg: &mut Config) -> Result<()> {
    validate_config(cfg)?;
    if cfg.session_token.is_empty() {
        return Err(Error::Config(
            "RMIG_SESSION_TOKEN is required in the process environment for rmigd; the token is not read from config.toml".into(),
        ));
    }
    Ok(())
}

/// Fills `cfg.sql_base` from `sql_root` if empty, and sets `cfg.database` when the catalog has exactly one database.
fn normalize_catalog_paths(cfg: &mut Config) -> Result<()> {
    if cfg.sql_base.is_empty() {
        cfg.sql_base = cfg.sql_root.clone();
    }
    let dbs = discover_catalog_databases(&cfg.sql_root)?;
    if cfg.database.is_empty() && dbs.len() == 1 {
        cfg.database = dbs[0].clone();
    }
    Ok(())
}

fn reject_control_chars(field: &str, value: &str) -> Result<()> {
    if value.chars().any(|c| c.is_control()) {
        return Err(Error::Config(format!(
            "{field} contains control characters"
        )));
    }
    Ok(())
}

#[path = "../tests/validate_test.rs"]
#[cfg(test)]
mod tests;
