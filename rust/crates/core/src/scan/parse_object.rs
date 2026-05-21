use std::path::Path;

use sha2::{Digest, Sha256};

use crate::domain::{share, ObjectEntry, ObjectKey, Script, ScriptKey, ScriptKind, StrOff};
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
    let script = Script {
        key: script_key,
        kind: ScriptKind::Object,
        abs_path: share(abs.to_string_lossy().as_ref()),
        checksum: Some(cs),
        scaffold: false,
    };
    let obj = ObjectEntry {
        key_off: StrOff::EMPTY,
        staging_key: Some(key),
        script_id: 0,
        checksum: cs,
        db_exists: false,
        db_id: 0,
    };
    Ok(Some((obj, script)))
}

fn file_checksum(path: &Path) -> Result<[u8; 32]> {
    let data = std::fs::read(path).map_err(crate::error::Error::Io)?;
    Ok(Sha256::digest(&data).into())
}
