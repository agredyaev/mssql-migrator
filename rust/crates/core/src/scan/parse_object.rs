use std::path::Path;

use sha2::{Digest, Sha256};

use crate::domain::{empty_str, share, ObjectEntry, ObjectKey, Script, ScriptKey, ScriptKind};
use crate::error::Result;

const KINDS: &[&str] = &[
    "tables",
    "views",
    "procedures",
    "functions",
    "triggers",
    "indexes",
    "types",
    "sequences",
    "synonyms",
];

pub fn parse_object(rel: &str, abs: &Path) -> Result<Option<(ObjectEntry, Script)>> {
    let parts: Vec<_> = rel.split('/').collect();
    if parts.len() < 4 {
        return Ok(None);
    }
    let name = parts[parts.len() - 1].trim_end_matches(".sql");
    let kind = parts[parts.len() - 2];
    if !KINDS.contains(&kind) {
        return Ok(None);
    }
    let schema = parts[parts.len() - 3];
    let database = share(parts[0]);
    crate::sql_ident::validate_path_component(database.as_ref())?;
    crate::sql_ident::validate_path_component(schema)?;
    crate::sql_ident::validate_path_component(name)?;
    let key = ObjectKey::new(schema, kind, name);
    let cs = file_checksum(abs)?;
    let script_key = ScriptKey::from_path(rel);
    let schema_s = share(schema);
    let kind_s = share(kind);
    let name_s = share(name);
    let script = Script {
        key: script_key.clone(),
        kind: ScriptKind::Object,
        abs_path: share(abs.to_string_lossy().as_ref()),
        schema: schema_s.clone(),
        object_kind: kind_s.clone(),
        object_name: name_s.clone(),
        checksum: Some(cs),
        git_hash: empty_str(),
        git_author: empty_str(),
        git_date: empty_str(),
        table_name: None,
        scaffold: false,
    };
    let obj = ObjectEntry {
        key: key.clone(),
        script: script_key,
        history: None,
        db: Default::default(),
        plan: None,
        checksum: cs,
        schema: schema_s,
        kind: kind_s,
        name: name_s,
        database_name: database,
        parent_name: empty_str(),
        parent_key: None,
    };
    Ok(Some((obj, script)))
}

fn file_checksum(path: &Path) -> Result<[u8; 32]> {
    let data = std::fs::read(path).map_err(crate::error::Error::Io)?;
    Ok(Sha256::digest(&data).into())
}
