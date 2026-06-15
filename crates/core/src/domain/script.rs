//! Script metadata: [`ScriptKind`], [`ScriptRow`], [`Script`] — dense scan output.
//!
//! ### Purpose
//! Represents a single `.sql` file discovered during the scan phase. The dense
//! [`ScriptRow`] replaces the scan-ingest [`Script`] after arena interning.

use super::key::ScriptKey;
use super::shared::SharedStr;
use super::str_off::StrOff;

/// Bit flag: script is a generated scaffold file.
pub const SCRIPT_FLAG_SCAFFOLD: u8 = 1 << 0;
/// Bit flag: script has an associated checksum.
pub const SCRIPT_FLAG_HAS_CHECKSUM: u8 = 1 << 1;

/// Classification of a script file in the SQL tree.
///
/// `#[repr(u8)]` — zero-cost numeric conversion for compact serialization
/// (`as_repr` / `from_repr`).
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

impl ScriptKind {
    /// Numeric representation (`u8`).
    pub const fn as_repr(self) -> u8 {
        self as u8
    }

    /// Decode from numeric representation.
    pub fn from_repr(v: u8) -> Option<Self> {
        match v {
            0 => Some(Self::Object),
            1 => Some(Self::Transition),
            2 => Some(Self::Check),
            _ => None,
        }
    }
}

/// Dense script row: kind + arena path offsets only.
#[derive(Clone, Debug)]
pub struct ScriptRow {
    /// Offset into arena for the relative path.
    pub path_off: StrOff,
    /// Offset into arena for the absolute path.
    pub abs_path_off: StrOff,
    /// Numeric kind (0=Object, 1=Transition, 2=Check).
    pub kind: u8,
    /// Bit-field: SCRIPT_FLAG_SCAFFOLD | SCRIPT_FLAG_HAS_CHECKSUM.
    pub flags: u8,
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
    /// True when this is a generated scaffold file.
    pub scaffold: bool,
}

impl Script {
    /// Consume `Script` into a dense `(ScriptRow, Option<checksum>)` pair.
    pub fn into_parts(self) -> (ScriptRow, Option<[u8; 32]>) {
        let mut flags = 0u8;
        if self.scaffold {
            flags |= SCRIPT_FLAG_SCAFFOLD;
        }
        let checksum = self.checksum;
        if checksum.is_some() {
            flags |= SCRIPT_FLAG_HAS_CHECKSUM;
        }
        let row = ScriptRow {
            path_off: StrOff::EMPTY,
            abs_path_off: StrOff::EMPTY,
            kind: self.kind.as_repr(),
            flags,
        };
        (row, checksum)
    }
}

impl ScriptRow {
    /// Decode the numeric kind into a [`ScriptKind`].
    pub fn kind(&self) -> ScriptKind {
        ScriptKind::from_repr(self.kind).unwrap_or(ScriptKind::Object)
    }

    /// True when the script is a generated scaffold.
    pub fn scaffold(&self) -> bool {
        self.flags & SCRIPT_FLAG_SCAFFOLD != 0
    }

    /// True when the script has an associated checksum.
    pub fn has_checksum(&self) -> bool {
        self.flags & SCRIPT_FLAG_HAS_CHECKSUM != 0
    }
}
