//! [`ObjectKey`] and [`ScriptKey`] — dense primary key types for domain objects.
//!
//! ### Purpose
//! `ObjectKey` is the canonical identifier for a database object
//! (`schema/kind/name`, lowercased, arena-backed). `ScriptKey` identifies a
//! script file by its relative path. Both wrap a single [`SharedStr`] for
//! cache-friendly equality and hashing.

use std::fmt;
use std::hash::{Hash, Hasher};

use serde::{Deserialize, Serialize};

use super::shared::{share, SharedStr};

#[path = "key_from.rs"]
mod key_from;

fn key_part(s: &str, index: usize) -> &str {
    let mut parts = s.split('/');
    parts.nth(index).unwrap_or("")
}

/// Normalised database-object key: `schema/kind/name` (lowercased, arena-backed).
///
/// Used as the primary key in `CatalogState`, `ChecksumMap`, and diff logic.
#[derive(Clone, Debug, Eq, Serialize, Deserialize)]
pub struct ObjectKey(SharedStr);

/// Script-file identifier: relative path (arena-backed).
#[derive(Clone, Debug, Eq, Serialize, Deserialize)]
pub struct ScriptKey(SharedStr);

impl PartialEq for ObjectKey {
    fn eq(&self, other: &Self) -> bool {
        self.0 == other.0
    }
}

impl Hash for ObjectKey {
    fn hash<H: Hasher>(&self, state: &mut H) {
        self.0.hash(state);
    }
}

impl PartialEq for ScriptKey {
    fn eq(&self, other: &Self) -> bool {
        self.0 == other.0
    }
}

impl Hash for ScriptKey {
    fn hash<H: Hasher>(&self, state: &mut H) {
        self.0.hash(state);
    }
}

impl ObjectKey {
    /// Build a key from raw parts (lowercased, concatenated with `/`).
    pub fn new(schema: &str, kind: &str, name: &str) -> Self {
        Self(share(format!(
            "{}/{}/{}",
            schema.to_lowercase(),
            kind.to_lowercase(),
            name.to_lowercase()
        )))
    }

    /// Build a key from an already-normalised `schema/kind/name` string.
    pub fn from_normalized(s: &str) -> Self {
        Self(share(s))
    }

    /// Parse a relative SQL file path (`<schema>/<kind>/<name>.sql`) into a key.
    ///
    /// Returns `None` when the path has fewer than 4 segments (database is
    /// stripped, then schema/kind/name).
    pub fn parse(path: &str) -> Option<Self> {
        let path = path.trim_end_matches(".sql");
        let parts: Vec<_> = path.split('/').collect();
        if parts.len() < 4 {
            return None;
        }
        let name = parts.last()?;
        let kind = parts[parts.len() - 2];
        let schema = parts[parts.len() - 3];
        Some(Self::new(schema, kind, name))
    }

    /// Raw `schema/kind/name` string.
    pub fn as_str(&self) -> &str {
        &self.0
    }

    /// `ChecksumMap` key (byte fingerprint via [`SharedStr::fingerprint`], no `as_str`).
    pub fn fingerprint(&self) -> u64 {
        self.0.fingerprint()
    }

    /// Schema segment of the key.
    pub fn schema_part(&self) -> &str {
        key_part(self.as_str(), 0)
    }

    /// Kind segment of the key.
    pub fn kind_part(&self) -> &str {
        key_part(self.as_str(), 1)
    }

    /// Object-name segment of the key.
    pub fn name_part(&self) -> &str {
        key_part(self.as_str(), 2)
    }

    /// Schema segment as a `SharedStr` subslice of the arena allocation.
    pub fn schema_shared(&self) -> SharedStr {
        SharedStr::subslice_of(&self.0, self.schema_part())
    }

    /// Kind segment as a `SharedStr` subslice.
    pub fn kind_shared(&self) -> SharedStr {
        SharedStr::subslice_of(&self.0, self.kind_part())
    }

    /// Name segment as a `SharedStr` subslice.
    pub fn name_shared(&self) -> SharedStr {
        SharedStr::subslice_of(&self.0, self.name_part())
    }

    /// Clone the inner `SharedStr`.
    pub fn shared(&self) -> SharedStr {
        self.0.clone()
    }
}

impl fmt::Display for ObjectKey {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(&self.0)
    }
}

impl ScriptKey {
    /// Build a script key from a relative path (normalises backslashes).
    pub fn from_path(path: &str) -> Self {
        Self(share(path.replace('\\', "/")))
    }

    /// Raw relative path string.
    pub fn as_str(&self) -> &str {
        &self.0
    }

    /// Clone the inner `SharedStr`.
    pub fn shared(&self) -> SharedStr {
        self.0.clone()
    }
}
