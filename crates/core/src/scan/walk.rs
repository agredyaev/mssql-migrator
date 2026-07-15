use std::collections::HashMap;
use std::ffi::OsStr;
use std::path::{Component, Path, PathBuf};

use crate::domain::Workspace;
use crate::error::Result;

use super::{parse, parse_parallel};

/// Walks `root`, parses all SQL schema files, and populates `ws`.
pub fn scan_root(ws: &mut Workspace, root: &str) -> Result<()> {
    ws.reset_layout();
    let root = Path::new(root)
        .canonicalize()
        .map_err(crate::error::Error::Io)?;
    ws.root = path_to_utf8(&root)?.into();
    let mut schemas = HashMap::new();
    let mut objects: Vec<(String, PathBuf)> = Vec::new();
    for entry in walk_sql(&root)? {
        let rel = relative_sql_path(&root, &entry)?;
        if rel.contains("/_migrations/") {
            parse::push_transition(ws, &rel, &entry)?;
        } else if rel.contains("/checks/") {
            parse::push_check(ws, &rel, &entry)?;
        } else {
            objects.push((rel, entry));
        }
    }
    // Read + checksum object files in parallel, then merge sequentially in order.
    for (i, parsed) in parse_parallel::parse_objects(&objects)?
        .into_iter()
        .enumerate()
    {
        parse::insert_parsed_object(ws, parsed, &objects[i].0, &mut schemas)?;
    }
    ws.schemas = schemas.into_values().collect();
    ws.schemas.sort_by(|a, b| a.name.cmp(&b.name));
    ws.finalize_object_layout();
    Ok(())
}

fn relative_sql_path(root: &Path, entry: &Path) -> Result<String> {
    let rel = entry.strip_prefix(root).map_err(|e| {
        crate::error::Error::InvalidInput(format!(
            "scan path outside root: root={} entry={} error={e}",
            root.display(),
            entry.display()
        ))
    })?;
    let mut out = Vec::new();
    for component in rel.components() {
        match component {
            Component::Normal(part) => out.push(os_str_to_utf8(part)?.to_string()),
            _ => {
                return Err(crate::error::Error::InvalidInput(format!(
                    "invalid scan path component: {}",
                    rel.display()
                )));
            }
        }
    }
    Ok(out.join("/"))
}

fn path_to_utf8(path: &Path) -> Result<&str> {
    path.to_str().ok_or_else(|| {
        crate::error::Error::InvalidInput(format!("path is not valid UTF-8: {}", path.display()))
    })
}

fn os_str_to_utf8(part: &OsStr) -> Result<&str> {
    part.to_str().ok_or_else(|| {
        crate::error::Error::InvalidInput(format!(
            "path component is not valid UTF-8: {}",
            part.to_string_lossy()
        ))
    })
}

fn walk_sql(root: &Path) -> Result<Vec<PathBuf>> {
    let mut out = Vec::new();
    walk_dir(root, &mut out)?;
    out.sort();
    Ok(out)
}

fn walk_dir(dir: &Path, out: &mut Vec<PathBuf>) -> Result<()> {
    for entry in std::fs::read_dir(dir).map_err(crate::error::Error::Io)? {
        let entry = entry.map_err(crate::error::Error::Io)?;
        let ft = entry.file_type().map_err(crate::error::Error::Io)?;
        let path = entry.path();
        if ft.is_symlink() {
            tracing::warn!(path = %path.display(), "skipping symlink (symlinks are not followed)");
            continue;
        }
        if path.is_dir() {
            walk_dir(&path, out)?;
        } else if path
            .extension()
            .and_then(|e| e.to_str())
            .is_some_and(|e| e.eq_ignore_ascii_case("sql"))
        {
            out.push(path);
        }
    }
    Ok(())
}
