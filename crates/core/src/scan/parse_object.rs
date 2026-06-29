use std::path::Path;

use sha2::{Digest, Sha256};

use crate::domain::{share, ObjectEntry, ObjectKey, Script, ScriptKey, ScriptKind, StrOff};
use crate::error::Result;

/// A parsed object script: key, dense entry, and script record. Built from
/// process-local `SharedStr`, so it can cross worker threads during parallel scan.
pub type ParsedObject = (ObjectKey, ObjectEntry, Script);

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

pub fn parse_object(rel: &str, abs: &Path) -> Result<Option<ParsedObject>> {
    let parts: Vec<_> = rel.split('/').collect();
    if parts.len() < 4 {
        return Ok(None);
    }
    let name = parts[parts.len() - 1].trim_end_matches(".sql");
    let kind = parts[parts.len() - 2];
    if !KINDS.contains(&kind) {
        // A file sitting exactly at `<db>/<schema>/<kind>/<name>.sql` whose `<kind>`
        // is not recognized is almost certainly a typo'd object-type folder. Warn so
        // it is not silently dropped from the plan. Deeper paths are left silent
        // (they may be intentional non-object content).
        if parts.len() == 4 {
            tracing::warn!(
                path = rel,
                kind = kind,
                "skipping file under unsupported object-type folder; expected one of: \
                 tables, views, procedures, functions, triggers, indexes, types, sequences, synonyms"
            );
        }
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
        script_id: 0,
        checksum: cs,
        flags: 0,
        db_id: 0,
    };
    Ok(Some((key, obj, script)))
}

fn file_checksum(path: &Path) -> Result<[u8; 32]> {
    let data = std::fs::read(path).map_err(crate::error::Error::Io)?;
    Ok(Sha256::digest(&data).into())
}
