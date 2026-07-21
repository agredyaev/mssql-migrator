use crate::error::Result;

use super::catalog::discover_catalog_databases;
use super::Config;

/// Fills `cfg.sql_base` from `sql_root` if empty, and sets `cfg.database` when the catalog has exactly one database.
pub fn normalize_catalog_paths(cfg: &mut Config) -> Result<()> {
    if cfg.sql_base.is_empty() {
        cfg.sql_base = cfg.sql_root.clone();
    }
    let dbs = discover_catalog_databases(&cfg.sql_root)?;
    if cfg.database.is_empty() && dbs.len() == 1 {
        cfg.database = dbs[0].clone();
    }
    Ok(())
}
