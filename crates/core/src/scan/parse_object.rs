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
    // Exactly `<db>/<schema>/<kind>/<name>.sql`: deeper paths (archives,
    // fixtures, backups) must never silently become live managed objects.
    if parts.len() > 4 {
        if KINDS.contains(&parts[parts.len() - 2]) {
            return Err(crate::error::Error::InvalidInput(format!(
                "object scripts must sit exactly at <database>/<schema>/<kind>/<name>.sql; \
                 nested path is not deployable: {rel}"
            )));
        }
        return Ok(None);
    }
    let name = strip_sql_ext(parts[parts.len() - 1]);
    let kind = parts[parts.len() - 2];
    if !KINDS.contains(&kind) {
        // A file sitting exactly at `<db>/<schema>/<kind>/<name>.sql` whose `<kind>`
        // is not recognized is almost certainly a typo'd object-type folder. Warn so
        // it is not silently dropped from the plan.
        tracing::warn!(
            path = rel,
            kind = kind,
            "skipping file under unsupported object-type folder; expected one of: \
             tables, views, procedures, functions, triggers, indexes, types, sequences, synonyms"
        );
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

/// Strips the `.sql` extension case-insensitively — the walker admits `.SQL`
/// too, and a lowercase-only strip would leak the extension into object keys.
pub(crate) fn strip_sql_ext(file: &str) -> &str {
    let n = file.len();
    if n >= 4 && file.is_char_boundary(n - 4) && file[n - 4..].eq_ignore_ascii_case(".sql") {
        &file[..n - 4]
    } else {
        file
    }
}

fn file_checksum(path: &Path) -> Result<[u8; 32]> {
    let data = std::fs::read(path).map_err(crate::error::Error::Io)?;
    Ok(content_checksum(&data))
}

/// Canonical script checksum. Apply-time re-verification must hash exactly the
/// way the scanner did, so both sides share this one function.
///
/// CRLF is folded to LF before hashing: a checkout-only line-ending flip is
/// not a semantic change and must not block deployment as table drift.
pub(crate) fn content_checksum(data: &[u8]) -> [u8; 32] {
    let mut h = Sha256::new();
    let mut rest = data;
    while let Some(pos) = rest.iter().position(|&b| b == b'\r') {
        if rest.get(pos + 1) == Some(&b'\n') {
            h.update(&rest[..pos]);
            h.update(b"\n");
            rest = &rest[pos + 2..];
        } else {
            h.update(&rest[..=pos]);
            rest = &rest[pos + 1..];
        }
    }
    h.update(rest);
    h.finalize().into()
}
