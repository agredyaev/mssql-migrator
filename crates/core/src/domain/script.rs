use super::key::ScriptKey;
use super::shared::SharedStr;
use super::str_off::StrOff;

pub const SCRIPT_FLAG_SCAFFOLD: u8 = 1 << 0;
pub const SCRIPT_FLAG_HAS_CHECKSUM: u8 = 1 << 1;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
#[repr(u8)]
pub enum ScriptKind {
    Object = 0,
    Transition = 1,
    Check = 2,
}

impl ScriptKind {
    pub const fn as_repr(self) -> u8 {
        self as u8
    }

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
    pub path_off: StrOff,
    pub abs_path_off: StrOff,
    pub kind: u8,
    pub flags: u8,
}

/// Git preload side table entry (cleared after arena intern).
#[derive(Clone, Debug, Default)]
pub struct ScriptGitStaging {
    pub hash: Option<SharedStr>,
    pub author: Option<SharedStr>,
    pub date: Option<SharedStr>,
}

/// Sparse git metadata keyed by 1-based script id.
#[derive(Clone, Debug, Default)]
pub struct ScriptGit {
    pub hash_off: StrOff,
    pub author_off: StrOff,
    pub date_off: StrOff,
}

/// Scan-ingest DTO; converted to [`ScriptRow`] in [`super::Workspace::insert_script`].
#[derive(Clone, Debug)]
pub struct Script {
    pub key: ScriptKey,
    pub kind: ScriptKind,
    pub abs_path: SharedStr,
    pub checksum: Option<[u8; 32]>,
    pub scaffold: bool,
}

impl Script {
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
    pub fn kind(&self) -> ScriptKind {
        ScriptKind::from_repr(self.kind).unwrap_or(ScriptKind::Object)
    }

    pub fn scaffold(&self) -> bool {
        self.flags & SCRIPT_FLAG_SCAFFOLD != 0
    }

    pub fn has_checksum(&self) -> bool {
        self.flags & SCRIPT_FLAG_HAS_CHECKSUM != 0
    }
}
