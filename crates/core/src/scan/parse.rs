use std::collections::HashMap;
use std::path::Path;

use crate::domain::{share, SchemaEntry, Workspace};
use crate::error::Result;

use super::parse_object::ParsedObject;

/// Merge a parsed object into the workspace (sequential: assigns `script_id`,
/// interns the database, records the schema). `parsed` is `None` for skipped files.
pub fn insert_parsed_object(
    ws: &mut Workspace,
    parsed: Option<ParsedObject>,
    rel: &str,
    schemas: &mut HashMap<String, SchemaEntry>,
) -> Result<()> {
    let Some((key, mut obj, script)) = parsed else {
        return Ok(());
    };
    let db = share(rel.split('/').next().unwrap_or(""));
    let schema_part = key.schema_part();
    schemas
        .entry(schema_part.to_string())
        .or_insert_with(|| SchemaEntry {
            database: db.clone(),
            name: share(schema_part),
            normalized: share(schema_part.to_lowercase()),
        });
    let script_id = ws.insert_script(script);
    obj.script_id = script_id;
    obj.db_id = ws.intern_database(db);
    ws.push_object(key, obj)?;
    Ok(())
}

pub fn push_transition(ws: &mut Workspace, rel: &str, abs: &Path) -> Result<()> {
    super::transition::ingest(ws, rel, abs)
}

pub fn push_check(ws: &mut Workspace, rel: &str, abs: &Path) -> Result<()> {
    let sk = crate::domain::ScriptKey::from_path(rel);
    ws.insert_script(crate::domain::Script {
        key: sk,
        kind: crate::domain::ScriptKind::Check,
        abs_path: share(abs.to_string_lossy().as_ref()),
        checksum: None,
        scaffold: false,
    });
    Ok(())
}
