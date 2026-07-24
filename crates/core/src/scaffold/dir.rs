use std::fs;
use std::io::Write;
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
        // Verify the deepest already-existing ancestor stays under base BEFORE
        // creating anything, so a symlinked ancestor (e.g. base/db -> elsewhere)
        // cannot materialize directory trees outside base and only then error.
        let mut existing = dir.as_path();
        while !existing.exists() {
            match existing.parent() {
                Some(p) => existing = p,
                None => break,
            }
        }
        if !existing
            .canonicalize()
            .map_err(Error::Io)?
            .starts_with(&base_canon)
        {
            return Err(Error::InvalidInput(
                "migration path escapes sql_base".into(),
            ));
        }
        fs::create_dir_all(&dir).map_err(Error::Io)?;
        let dir_canon = dir.canonicalize().map_err(Error::Io)?;
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
        let Ok(data) = crate::file_io::read_bounded(&path, crate::file_io::MAX_SQL_SCRIPT_BYTES)
        else {
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
    let mut file = fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .open(path)?;
    file.write_all(content.as_bytes())
}
