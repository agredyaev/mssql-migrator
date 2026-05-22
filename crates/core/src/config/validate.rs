use crate::error::{Error, Result};

use super::catalog::normalize_catalog_paths;
use super::Config;

pub fn validate_config(cfg: &mut Config) -> Result<()> {
    let mut missing = Vec::new();
    if cfg.server.is_empty() {
        missing.push("RM_DB_SERVER");
    }
    if cfg.sql_root.is_empty() {
        missing.push("RM_SQL_ROOT");
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
