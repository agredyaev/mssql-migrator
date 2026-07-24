use std::path::Path;

use crate::error::{Error, Result};

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
        if !ft.is_dir() {
            continue;
        }
        let name = entry.file_name().into_string().map_err(|_| {
            Error::InvalidInput("catalog database directory name is not valid UTF-8".into())
        })?;
        if name.starts_with('.') {
            continue;
        }
        crate::sql_ident::validate_path_component(&name)?;
        if catalog_database_dir_has_schema(entry.path().as_path())? {
            names.push(name);
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

fn catalog_database_dir_has_schema(db_dir: &Path) -> Result<bool> {
    // An unreadable candidate directory must surface as an error, never as
    // "not a database": permissions or mount failures would otherwise shrink
    // the deployment scope silently.
    let entries = std::fs::read_dir(db_dir).map_err(|e| {
        Error::Config(format!(
            "cannot read catalog candidate {}: {e}",
            db_dir.display()
        ))
    })?;
    for entry in entries {
        let entry = entry.map_err(|e| {
            Error::Config(format!("cannot read entry under {}: {e}", db_dir.display()))
        })?;
        let ft = entry
            .file_type()
            .map_err(|e| Error::Config(format!("cannot stat {}: {e}", entry.path().display())))?;
        if ft.is_dir() {
            return Ok(true);
        }
    }
    Ok(false)
}
