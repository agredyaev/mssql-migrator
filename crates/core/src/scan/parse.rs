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
    schemas: &mut HashMap<(String, String), SchemaEntry>,
) -> Result<()> {
    let Some((key, mut obj, script)) = parsed else {
        return Ok(());
    };
    let db = share(rel.split('/').next().unwrap_or(""));
    let schema_part = key.schema_part();
    // Keyed by (database, schema): the same schema name in two catalog
    // databases is two schemas — collapsing them drops CREATE SCHEMA for
    // every database after the first.
    schemas
        .entry((db.as_ref().to_string(), schema_part.to_string()))
        .or_insert_with(|| SchemaEntry {
            database: db.clone(),
            name: share(schema_part),
            normalized: share(schema_part.to_lowercase()),
        });
    let script_id = ws.insert_script(script);
    obj.script_id = script_id;
    if ws.database_count() >= u16::MAX as usize - 1 {
        return Err(crate::error::Error::InvalidInput(
            "too many distinct database directories under RM_SQL_ROOT (max 65534)".into(),
        ));
    }
    obj.db_id = ws.intern_database(db);
    ws.push_object(key, obj)?;
    Ok(())
}

pub fn push_check(ws: &mut Workspace, rel: &str, abs: &Path) -> Result<()> {
    let sk = crate::domain::ScriptKey::from_path(rel);
    ws.insert_script(crate::domain::Script {
        key: sk,
        kind: crate::domain::ScriptKind::Check,
        abs_path: share(abs.to_string_lossy().as_ref()),
        checksum: None,
    });
    Ok(())
}

/// True only for files directly inside a `checks/` folder at its contract
/// positions: `<db>/checks/<file>.sql` or `<db>/<schema>/checks/<file>.sql`.
/// Deeper paths (for example a schema literally named `checks` holding kind
/// folders) stay ordinary objects.
pub fn is_check_path(rel: &str) -> bool {
    let parts: Vec<&str> = rel.split('/').collect();
    matches!(
        parts.as_slice(),
        [_, "checks", f] | [_, _, "checks", f] if !f.is_empty()
    )
}
