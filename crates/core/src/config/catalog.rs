//! Catalog layout: database names are top-level directories under `RM_SQL_ROOT`
//! (e.g. `sql_root/dactests/smoke/tables/...` → database `dactests`).

use std::path::Path;

use crate::driver::{connect, mssql};
use crate::error::{Error, Result};

use super::Config;

/// Immediate child directories of `sql_root` that contain at least one schema subdirectory.
pub fn discover_catalog_databases(sql_root: &str) -> Result<Vec<String>> {
    let root = Path::new(sql_root);
    if !root.is_dir() {
        return Err(Error::Config(format!(
            "RM_SQL_ROOT is not a directory: {}",
            root.display()
        )));
    }
    let mut names = Vec::new();
    for entry in std::fs::read_dir(root).map_err(Error::Io)? {
        let entry = entry.map_err(Error::Io)?;
        let ft = entry.file_type().map_err(Error::Io)?;
        if !ft.is_dir() || ft.is_symlink() {
            continue;
        }
        let name = entry.file_name();
        let name = name.to_string_lossy();
        if name.starts_with('.') {
            continue;
        }
        if catalog_database_dir_has_schema(entry.path().as_path()) {
            names.push(name.into_owned());
        }
    }
    names.sort();
    if names.is_empty() {
        return Err(Error::Config(format!(
            "no catalog databases under {} (expected <sql_root>/<database>/<schema>/...)",
            root.display()
        )));
    }
    Ok(names)
}

fn catalog_database_dir_has_schema(db_dir: &Path) -> bool {
    let Ok(entries) = std::fs::read_dir(db_dir) else {
        return false;
    };
    for entry in entries.flatten() {
        if entry
            .file_type()
            .map(|ft| ft.is_dir() && !ft.is_symlink())
            .unwrap_or(false)
        {
            return true;
        }
    }
    false
}

/// Default `sql_base` to `sql_root`; set `database` when exactly one catalog DB exists.
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

/// Resolve `cfg.database` from catalog (single DB) or verify it exists under `sql_root`.
pub fn resolve_single_database(cfg: &mut Config) -> Result<()> {
    normalize_catalog_paths(cfg)?;
    let dbs = discover_catalog_databases(&cfg.sql_root)?;
    if cfg.database.is_empty() {
        if dbs.len() == 1 {
            cfg.database = dbs[0].clone();
            return Ok(());
        }
        return Err(Error::Config(format!(
            "catalog has multiple databases {dbs:?}; use one database directory per RM_SQL_ROOT"
        )));
    }
    if !dbs.iter().any(|d| d == &cfg.database) {
        return Err(Error::Config(format!(
            "database {:?} not found under {} (catalog: {dbs:?})",
            cfg.database, cfg.sql_root
        )));
    }
    Ok(())
}

pub async fn ensure_catalog_databases_exist(cfg: &Config, names: &[String]) -> Result<()> {
    if names.is_empty() {
        return Ok(());
    }
    let mut master = cfg.clone();
    master.database = "master".into();
    let mut conn = connect(&master).await?;
    for db in names {
        let escaped = db.replace('\'', "''");
        let bracket = db.replace(']', "]]");
        let sql = format!("IF DB_ID(N'{escaped}') IS NULL CREATE DATABASE [{bracket}]");
        mssql::exec(&mut conn.client, &sql).await?;
    }
    Ok(())
}

#[cfg(test)]
#[path = "tests/catalog.rs"]
mod catalog_tests;
