//! Transition script ingestion: parses `_migrations/` paths into [`Workspace`](crate::domain) entries.

use std::path::Path;

use crate::domain::{share, ObjectKey, Script, ScriptKey, ScriptKind, Workspace};
use crate::error::Result;

const SCAFFOLD: &str = "-- rmig: transition-scaffold";

/// `history.normalized_key` is NVARCHAR(512): a longer key would be silently
/// truncated by SQL Server and the full path re-executed on every run.
const MAX_KEY_UTF16_UNITS: usize = 512;

/// Parses and registers one transition script file into `ws`.
pub fn ingest(ws: &mut Workspace, rel: &str, abs: &Path) -> Result<()> {
    let Some(meta) = parse_meta(rel)? else {
        tracing::warn!(
            path = rel,
            "ignoring file under _migrations/: expected <schema>/tables/_migrations/<table>/<3-digit-ordinal>_<commit>_<slug>.sql"
        );
        return Ok(());
    };
    let data = std::fs::read(abs).map_err(crate::error::Error::Io)?;
    let cs: [u8; 32] = super::content_checksum(&data);
    let scaffold = is_scaffold(&data);
    let sk = ScriptKey::from_path(&meta.path);
    ws.insert_script(Script {
        key: sk.clone(),
        kind: ScriptKind::Transition,
        abs_path: share(abs.to_string_lossy().as_ref()),
        checksum: Some(cs),
    });
    if !scaffold {
        let database = share(rel.split('/').next().unwrap_or(""));
        ws.push_transition_staging(database, meta.table_key.clone(), share(&meta.ordinal), sk)?;
        ws.invalidate_transition_paths();
    }
    Ok(())
}

struct TransitionMeta {
    table_key: ObjectKey,
    path: String,
    ordinal: String,
}

fn parse_meta(rel: &str) -> Result<Option<TransitionMeta>> {
    let parts: Vec<_> = rel.split('/').collect();
    if parts.len() < 6 {
        return Ok(None);
    }
    let n = parts.len();
    if parts[n - 4] != "tables" || parts[n - 3] != "_migrations" {
        return Ok(None);
    }
    let schema = parts[n - 5].to_string();
    let table = parts[n - 2].to_string();
    let file = parts[n - 1];
    crate::sql_ident::validate_path_component(&schema)?;
    crate::sql_ident::validate_path_component(&table)?;
    crate::sql_ident::validate_path_component(file)?;
    let Some(ordinal) = parse_filename(file) else {
        return Ok(None);
    };
    if rel.encode_utf16().count() > MAX_KEY_UTF16_UNITS {
        return Err(crate::error::Error::InvalidInput(format!(
            "transition path exceeds {MAX_KEY_UTF16_UNITS} characters and cannot be \
             recorded exactly in audit history: {rel}"
        )));
    }
    let path = rel.to_string();
    Ok(Some(TransitionMeta {
        table_key: ObjectKey::new(&schema, "tables", &table),
        path,
        ordinal,
    }))
}

fn parse_filename(file: &str) -> Option<String> {
    let stripped = super::strip_sql_ext(file);
    if stripped.len() == file.len() {
        return None;
    }
    let file = stripped;
    let (ordinal, rest) = file.split_once('_')?;
    if ordinal.len() != 3 || !ordinal.chars().all(|c| c.is_ascii_digit()) {
        return None;
    }
    let (commit, slug) = rest.split_once('_')?;
    if commit.len() < 7 || !commit.chars().all(|c| c.is_ascii_hexdigit()) {
        return None;
    }
    if slug.is_empty() {
        return None;
    }
    Some(ordinal.into())
}

#[cfg(test)]
#[path = "../tests/scan_transition_test.rs"]
mod scan_transition_tests;

fn is_scaffold(data: &[u8]) -> bool {
    let line = data.split(|&b| b == b'\n').next().unwrap_or(&[]);
    let line = std::str::from_utf8(line)
        .unwrap_or("")
        .trim_end_matches('\r');
    line.starts_with(SCAFFOLD)
}
