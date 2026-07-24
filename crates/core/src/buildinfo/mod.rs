//! Compile-time version metadata extraction and serialization.
//!
//! ### Purpose
//! Emits version and git commit metadata. This enables runtime diagnostic trace
//! and guarantees that execution reports (JSON) and CLI version queries have
//! exact, traceable software identity.
//!
//! ### Implementation
//! Cargo exposes the package version as `CARGO_PKG_VERSION`. The `build.rs`
//! script resolves the current git commit with `git rev-parse --short HEAD`
//! and exports it as `RMIG_COMMIT`.
//!
//! A non-git source tree uses `unknown` for the commit.

use std::io::Write;

use crate::error::{Error, Result};

/// Package version from `[workspace.package]` in the root `Cargo.toml`.
pub fn version() -> &'static str {
    env!("CARGO_PKG_VERSION")
}

/// Sourced git commit short hash from compile-time `RMIG_COMMIT` or returns `unknown`.
pub fn commit() -> &'static str {
    match option_env!("RMIG_COMMIT") {
        Some(c) if !c.trim().is_empty() && c.trim() != "unknown" => c.trim(),
        _ => "unknown",
    }
}

/// Sourced author name of the application.
pub fn author() -> &'static str {
    "Aleksey Gredyaev"
}

/// Emits human-readable single-line version summary.
pub fn summary() -> String {
    format!("rmig {} {} by {}", version(), commit(), author())
}

/// Serializes compile-time metadata into JSON format (`{"version": "...", "commit": "...", "author": "..."}`).
pub fn write_json(mut w: impl Write) -> Result<()> {
    let obj = serde_json::json!({
        "version": version(),
        "commit": commit(),
        "author": author(),
    });
    serde_json::to_writer(&mut w, &obj).map_err(|e| Error::InvalidInput(e.to_string()))?;
    w.write_all(b"\n")
        .map_err(|e| Error::InvalidInput(e.to_string()))?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn summary_has_rmig_prefix() {
        let s = summary();
        assert!(s.starts_with("rmig "), "summary = {s:?}");
    }

    #[test]
    fn version_non_empty() {
        assert!(!version().is_empty());
    }

    #[test]
    fn commit_non_empty() {
        assert!(!commit().is_empty());
    }

    #[test]
    fn write_json_round_trip() {
        let mut buf = Vec::new();
        write_json(&mut buf).expect("write_json");
        let v: serde_json::Value = serde_json::from_slice(&buf).expect("json");
        assert!(v
            .get("version")
            .and_then(|x| x.as_str())
            .is_some_and(|s| !s.is_empty()));
        assert!(v
            .get("commit")
            .and_then(|x| x.as_str())
            .is_some_and(|s| !s.is_empty()));
        assert_eq!(
            v.get("author").and_then(|x| x.as_str()),
            Some("Aleksey Gredyaev")
        );
    }
}
