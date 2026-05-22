//! Catalog layout helpers for integration tests.
//!
//! Database name comes from top-level directories under `RM_SQL_ROOT`, not from env/config.

use migrator_core::config::discover_catalog_databases;
use migrator_core::error::{Error, Result};

/// Sole catalog database directory under `sql_root` (mirrors production layout discovery).
pub fn sole_catalog_database(sql_root: &str) -> Result<String> {
    let dbs = discover_catalog_databases(sql_root)?;
    if dbs.len() != 1 {
        return Err(Error::Config(format!(
            "integration fixture expects one catalog database, found {dbs:?} under {sql_root}"
        )));
    }
    Ok(dbs[0].clone())
}

/// Path relative to `sql_root`: `{catalog_db}/{rel}`.
pub fn catalog_sql_rel(sql_root: &str, rel: &str) -> Result<String> {
    Ok(format!("{}/{}", sole_catalog_database(sql_root)?, rel))
}
