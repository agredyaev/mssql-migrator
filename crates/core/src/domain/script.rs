//! SQL script metadata.

use super::key::ScriptKey;

/// Classification of a script file in the SQL tree.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
#[repr(u8)]
pub enum ScriptKind {
    /// Regular object script.
    Object = 0,
    /// Ordered table transition.
    Transition = 1,
    /// Pre-condition check.
    Check = 2,
}

/// Stored script metadata.
#[derive(Clone, Debug)]
pub struct ScriptRow {
    /// Repository-relative path.
    pub key: ScriptKey,
    /// Absolute filesystem path.
    pub abs_path: String,
    /// Script classification.
    pub kind: ScriptKind,
}

/// Optional Git metadata keyed by script id.
#[derive(Clone, Debug, Default)]
pub struct ScriptGit {
    /// Git commit hash.
    pub hash: Option<String>,
    /// Git author.
    pub author: Option<String>,
    /// ISO-8601 commit date.
    pub date: Option<String>,
}

/// Script discovered during scan.
#[derive(Clone, Debug)]
pub struct Script {
    /// Repository-relative path key.
    pub key: ScriptKey,
    /// Script classification.
    pub kind: ScriptKind,
    /// Absolute filesystem path.
    pub abs_path: String,
    /// Optional SHA-256 checksum.
    pub checksum: Option<[u8; 32]>,
}

impl Script {
    /// Consumes the scan value into a stored row and checksum.
    pub fn into_parts(self) -> (ScriptRow, Option<[u8; 32]>) {
        (
            ScriptRow {
                key: self.key,
                abs_path: self.abs_path,
                kind: self.kind,
            },
            self.checksum,
        )
    }
}
