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
