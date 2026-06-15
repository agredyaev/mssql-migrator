use crate::error::{Error, Result};

use super::auth_mode::sql_credentials_required;
use super::catalog_paths::normalize_catalog_paths;
use super::Config;

pub fn validate_config(cfg: &mut Config) -> Result<()> {
    let mut missing = Vec::new();
    let requires_sql_credentials = sql_credentials_required(&cfg.db_auth);
    if cfg.server.is_empty() {
        missing.push("RM_DB_SERVER");
    }
    if cfg.sql_root.is_empty() {
        missing.push("RM_SQL_ROOT");
    }
    if requires_sql_credentials {
        if cfg.user.is_empty() {
            missing.push("RM_DB_USER");
        }
        if cfg.password.is_empty() {
            missing.push("RM_DB_PASSWORD");
        }
    }
    if !missing.is_empty() {
        return Err(Error::Config(format!(
            "missing required variables: {}",
            missing.join(", ")
        )));
    }
    normalize_catalog_paths(cfg)?;
    Ok(())
}

#[path = "validate_test.rs"]
#[cfg(test)]
mod tests;
