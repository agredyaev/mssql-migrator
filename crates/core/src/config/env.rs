use std::collections::HashMap;
use std::path::Path;

use crate::error::{Error, Result};

#[cfg(unix)]
fn warn_env_file_permissions(path: &Path) {
    use std::os::unix::fs::PermissionsExt;
    if let Ok(meta) = std::fs::metadata(path) {
        let mode = meta.permissions().mode();
        if mode & 0o077 != 0 {
            eprintln!(
                "warning: env file {} is readable by group/other (mode {:o}); restrict to 0600",
                path.display(),
                mode & 0o777
            );
        }
    }
}

#[cfg(not(unix))]
fn warn_env_file_permissions(_path: &Path) {}

/// Strips at most ONE matching surrounding quote pair, so a credential whose
/// value legitimately begins or ends with a quote is not silently mangled.
fn strip_one_quote_pair(v: &str) -> &str {
    for q in ['"', '\''] {
        if v.len() >= 2 && v.starts_with(q) && v.ends_with(q) {
            return &v[1..v.len() - 1];
        }
    }
    v
}

/// Parses `path` as a dotenv file and returns key/value pairs; returns an empty map when the file is absent.
pub fn load_env_file(path: &Path) -> Result<HashMap<String, String>> {
    load_env_file_inner(path, false)
}

/// Parses `path` as a dotenv file and errors when the file is absent.
pub fn load_env_file_required(path: &Path) -> Result<HashMap<String, String>> {
    load_env_file_inner(path, true)
}

fn load_env_file_inner(path: &Path, required: bool) -> Result<HashMap<String, String>> {
    if !path.is_file() {
        if required {
            return Err(Error::Config(format!(
                "env file not found: {}",
                path.display()
            )));
        }
        return Ok(HashMap::new());
    }
    warn_env_file_permissions(path);
    let content = std::fs::read_to_string(path)
        .map_err(|e| Error::Config(format!("env file unreadable: {}: {e}", path.display())))?;
    let mut env = HashMap::new();
    for line in content.lines() {
        let line = line.trim();
        if line.is_empty() || line.starts_with('#') {
            continue;
        }
        let Some((k, v)) = line.split_once('=') else {
            eprintln!("warning: ignoring malformed env line (no '='): {line}");
            continue;
        };
        let v = strip_one_quote_pair(v.trim());
        env.insert(k.trim().to_string(), v.to_string());
    }
    Ok(env)
}

#[cfg(test)]
#[path = "../tests/env_test.rs"]
mod env_tests;
