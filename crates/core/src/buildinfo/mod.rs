//! Compile-time version metadata extraction and serialization.
//!
//! ### Purpose
//! Emits version and git commit metadata. This enables runtime diagnostic trace
//! and guarantees that execution reports (JSON) and CLI version queries have
//! exact, traceable software identity.
//!
//! ### Implementation
//! The `build.rs` script runs at compile-time to resolve:
//! 1. The software version from the root `VERSION` file, exporting it as `RMIG_VERSION`.
//! 2. The current git commit hash via `git rev-parse --short HEAD`, exporting it as `RMIG_COMMIT`.
//!
//! If these variables are not set (e.g., during off-line non-git builds), they fallback to
//! `0.0.0-dev` and `unknown` respectively.

use std::io::Write;

use crate::error::{Error, Result};

/// Sourced version from compile-time `RMIG_VERSION` or returns default `0.0.0-dev`.
pub fn version() -> &'static str {
    match option_env!("RMIG_VERSION") {
        Some(v) if !v.trim().is_empty() => v.trim(),
        _ => "0.0.0-dev",
    }
}

/// Sourced git commit short hash from compile-time `RMIG_COMMIT` or returns `unknown`.
pub fn commit() -> &'static str {
    match option_env!("RMIG_COMMIT") {
        Some(c) if !c.trim().is_empty() && c.trim() != "unknown" => short_rev(c.trim()),
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

fn short_rev(s: &str) -> &str {
    let s = s.strip_prefix("vcs:").unwrap_or(s).trim();
    if s.len() <= 7 {
        s
    } else {
        &s[..7]
    }
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

    #[test]
    fn short_rev_truncates_long_hash() {
        assert_eq!(short_rev("abcdef1234567890"), "abcdef1");
    }
}
