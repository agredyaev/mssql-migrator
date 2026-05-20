use std::collections::HashMap;
use std::path::Path;

use crate::domain::{empty_str, share, SchemaEntry, Workspace};
use crate::error::Result;

use super::parse_object;

pub fn ingest_object(
    ws: &mut Workspace,
    rel: &str,
    abs: &Path,
    schemas: &mut HashMap<String, SchemaEntry>,
) -> Result<()> {
    let Some((obj, script)) = parse_object::parse_object(rel, abs)? else {
        return Ok(());
    };
    let db = share(rel.split('/').next().unwrap_or(""));
    schemas
        .entry(obj.schema.as_ref().to_string())
        .or_insert_with(|| SchemaEntry {
            database: db,
            name: obj.schema.clone(),
            normalized: share(obj.schema.as_ref().to_lowercase()),
        });
    ws.scripts.insert(script.key.clone(), script);
    ws.push_object(obj);
    Ok(())
}

pub fn push_transition(ws: &mut Workspace, rel: &str, abs: &Path) -> Result<()> {
    super::transition::ingest(ws, rel, abs)
}

pub fn push_check(ws: &mut Workspace, rel: &str, abs: &Path) -> Result<()> {
    let sk = crate::domain::ScriptKey::from_path(rel);
    ws.scripts.insert(
        sk.clone(),
        crate::domain::Script {
            key: sk,
            kind: crate::domain::ScriptKind::Check,
            abs_path: share(abs.to_string_lossy().as_ref()),
            schema: crate::domain::empty_str(),
            object_kind: crate::domain::share("check"),
            object_name: crate::domain::empty_str(),
            checksum: None,
            git_hash: empty_str(),
            git_author: empty_str(),
            git_date: empty_str(),
            table_name: None,
            scaffold: false,
        },
    );
    Ok(())
}
