//! Script metadata: [`ScriptKind`], [`ScriptRow`], [`Script`] — dense scan output.
//!
//! ### Purpose
//! Represents a single `.sql` file discovered during the scan phase. The dense
//! [`ScriptRow`] replaces the scan-ingest [`Script`] after arena interning.

use super::key::ScriptKey;
use super::shared::SharedStr;
use super::str_off::StrOff;

/// Classification of a script file in the SQL tree.
///
/// `#[repr(u8)]` keeps the enum a single byte so [`ScriptRow`] stays dense.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
#[repr(u8)]
pub enum ScriptKind {
    /// Regular object script (`<schema>/<kind>/<name>.sql`).
    Object = 0,
    /// Transition script (`_migrations/<name>.sql`).
    Transition = 1,
    /// Check script (pre-condition validation).
    Check = 2,
}

/// Dense script row: kind + arena path offsets only.
#[derive(Clone, Debug)]
pub struct ScriptRow {
    /// Offset into arena for the relative path.
    pub path_off: StrOff,
    /// Offset into arena for the absolute path.
    pub abs_path_off: StrOff,
    /// Script classification.
    pub kind: ScriptKind,
}

/// Git preload side table entry (cleared after arena intern).
#[derive(Clone, Debug, Default)]
pub struct ScriptGitStaging {
    /// Git commit hash.
    pub hash: Option<SharedStr>,
    /// Git author `<name> <<email>>`.
    pub author: Option<SharedStr>,
    /// ISO-8601 commit date.
    pub date: Option<SharedStr>,
}

/// Sparse git metadata keyed by 1-based script id.
#[derive(Clone, Debug, Default)]
pub struct ScriptGit {
    /// Arena offset for the git hash.
    pub hash_off: StrOff,
    /// Arena offset for the git author.
    pub author_off: StrOff,
    /// Arena offset for the git date.
    pub date_off: StrOff,
}

/// Scan-ingest DTO; converted to [`ScriptRow`] in [`super::Workspace::insert_script`].
#[derive(Clone, Debug)]
pub struct Script {
    /// Script file key.
    pub key: ScriptKey,
    /// Script classification.
    pub kind: ScriptKind,
    /// Absolute filesystem path.
    pub abs_path: SharedStr,
    /// Optional SHA-256 checksum.
    pub checksum: Option<[u8; 32]>,
}

impl Script {
    /// Consume `Script` into a dense `(ScriptRow, Option<checksum>)` pair.
    pub fn into_parts(self) -> (ScriptRow, Option<[u8; 32]>) {
        let row = ScriptRow {
            path_off: StrOff::EMPTY,
            abs_path_off: StrOff::EMPTY,
            kind: self.kind,
        };
        (row, self.checksum)
    }
}
