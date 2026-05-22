use std::collections::HashMap;
use std::path::{Path, PathBuf};

use crate::domain::Workspace;
use crate::error::Result;

use super::parse;

pub fn scan_root(ws: &mut Workspace, root: &str) -> Result<()> {
    ws.reset_layout();
    let root = Path::new(root)
        .canonicalize()
        .map_err(crate::error::Error::Io)?;
    ws.root = root.to_string_lossy().into();
    let mut schemas = HashMap::new();
    for entry in walk_sql(&root)? {
        let rel = entry
            .strip_prefix(&root)
            .unwrap()
            .to_string_lossy()
            .replace('\\', "/");
        if rel.contains("/_migrations/") {
            parse::push_transition(ws, &rel, &entry)?;
            continue;
        }
        if rel.contains("/checks/") {
            parse::push_check(ws, &rel, &entry)?;
            continue;
        }
        parse::ingest_object(ws, &rel, &entry, &mut schemas)?;
    }
    ws.schemas = schemas.into_values().collect();
    ws.schemas.sort_by(|a, b| a.name.cmp(&b.name));
    ws.finalize_object_layout();
    Ok(())
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
        if ft.is_symlink() {
            continue;
        }
        let path = entry.path();
        if path.is_dir() {
            walk_dir(&path, out)?;
        } else if path.extension().and_then(|e| e.to_str()) == Some("sql") {
            out.push(path);
        }
    }
    Ok(())
}
