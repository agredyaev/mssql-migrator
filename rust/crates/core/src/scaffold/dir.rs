use std::fs;
use std::path::{Path, PathBuf};

use crate::error::{Error, Result};
use crate::sql_ident::validate_path_component;

pub const SCAFFOLD_MARK: &str = "-- rmig: transition-scaffold";

pub fn migration_dir(base: &Path, db: &str, schema: &str, table: &str) -> PathBuf {
    base.join(db)
        .join(schema)
        .join("tables")
        .join("_migrations")
        .join(table)
}

/// Validated migration directory; must resolve under `base` when `base` exists.
pub fn migration_dir_checked(base: &Path, db: &str, schema: &str, table: &str) -> Result<PathBuf> {
    validate_path_component(db)?;
    validate_path_component(schema)?;
    validate_path_component(table)?;
    let dir = migration_dir(base, db, schema, table);
    if base.exists() {
        let base_canon = base.canonicalize().map_err(Error::Io)?;
        let dir_canon = dir.canonicalize().or_else(|_| {
            fs::create_dir_all(&dir).map_err(Error::Io)?;
            dir.canonicalize().map_err(Error::Io)
        })?;
        if !dir_canon.starts_with(&base_canon) {
            return Err(Error::InvalidInput(format!(
                "migration path escapes sql_base: {}",
                dir_canon.display()
            )));
        }
        return Ok(dir_canon);
    }
    Ok(dir)
}

pub fn has_non_scaffold_sql(dir: &Path) -> bool {
    let Ok(entries) = fs::read_dir(dir) else {
        return false;
    };
    for ent in entries.flatten() {
        let path = ent.path();
        if path.extension().and_then(|e| e.to_str()) != Some("sql") {
            continue;
        }
        let Ok(data) = fs::read(&path) else {
            continue;
        };
        let line = data.split(|&b| b == b'\n').next().unwrap_or(&[]);
        let line = std::str::from_utf8(line)
            .unwrap_or("")
            .trim_end_matches('\r');
        if !line.starts_with(SCAFFOLD_MARK) {
            return true;
        }
    }
    false
}

pub fn write_file(path: &Path, content: &str) -> std::io::Result<()> {
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent)?;
    }
    fs::write(path, content)
}
