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

#[derive(Clone, Debug, Eq, Serialize, Deserialize)]
pub struct ObjectKey(SharedStr);

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
    pub fn new(schema: &str, kind: &str, name: &str) -> Self {
        Self(share(format!(
            "{}/{}/{}",
            schema.to_lowercase(),
            kind.to_lowercase(),
            name.to_lowercase()
        )))
    }

    pub fn from_normalized(s: &str) -> Self {
        Self(share(s))
    }

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

    pub fn as_str(&self) -> &str {
        &self.0
    }

    /// [`crate::db::ChecksumMap`] key (byte fingerprint, no `as_str`).
    pub fn fingerprint(&self) -> u64 {
        self.0.fingerprint()
    }

    pub fn schema_part(&self) -> &str {
        key_part(self.as_str(), 0)
    }

    pub fn kind_part(&self) -> &str {
        key_part(self.as_str(), 1)
    }

    pub fn name_part(&self) -> &str {
        key_part(self.as_str(), 2)
    }

    pub fn schema_shared(&self) -> SharedStr {
        SharedStr::subslice_of(&self.0, self.schema_part())
    }

    pub fn kind_shared(&self) -> SharedStr {
        SharedStr::subslice_of(&self.0, self.kind_part())
    }

    pub fn name_shared(&self) -> SharedStr {
        SharedStr::subslice_of(&self.0, self.name_part())
    }

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
    pub fn from_path(path: &str) -> Self {
        Self(share(path.replace('\\', "/")))
    }

    pub fn as_str(&self) -> &str {
        &self.0
    }

    pub fn shared(&self) -> SharedStr {
        self.0.clone()
    }
}
