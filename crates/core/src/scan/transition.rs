use std::path::Path;

use sha2::{Digest, Sha256};

use crate::domain::{share, ObjectKey, Script, ScriptKey, ScriptKind, Workspace};
use crate::error::Result;

const SCAFFOLD: &str = "-- rmig: transition-scaffold";

pub fn ingest(ws: &mut Workspace, rel: &str, abs: &Path) -> Result<()> {
    let Some(meta) = parse_meta(rel)? else {
        return Ok(());
    };
    let data = std::fs::read(abs).map_err(crate::error::Error::Io)?;
    let cs: [u8; 32] = Sha256::digest(&data).into();
    let scaffold = is_scaffold(abs);
    let sk = ScriptKey::from_path(&meta.path);
    ws.insert_script(Script {
        key: sk.clone(),
        kind: ScriptKind::Transition,
        abs_path: share(abs.to_string_lossy().as_ref()),
        checksum: Some(cs),
        scaffold,
    });
    if !scaffold {
        ws.push_transition_staging(meta.table_key.clone(), share(&meta.ordinal), sk);
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
    let rel = rel.replace('\\', "/");
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
    let Some((ordinal, _, _)) = parse_filename(file) else {
        return Ok(None);
    };
    let path = rel;
    Ok(Some(TransitionMeta {
        table_key: ObjectKey::new(&schema, "tables", &table),
        path,
        ordinal,
    }))
}

fn parse_filename(file: &str) -> Option<(String, String, String)> {
    let file = file.strip_suffix(".sql")?;
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
    Some((ordinal.into(), commit.into(), slug.into()))
}

fn is_scaffold(path: &Path) -> bool {
    let Ok(data) = std::fs::read(path) else {
        return false;
    };
    let line = data.split(|&b| b == b'\n').next().unwrap_or(&[]);
    let line = std::str::from_utf8(line)
        .unwrap_or("")
        .trim_end_matches('\r');
    line.starts_with(SCAFFOLD)
}
