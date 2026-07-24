//! Normalized object and script keys.

use std::{borrow::Borrow, fmt};

use serde::{Deserialize, Serialize};

fn key_part(value: &str, index: usize) -> &str {
    value.split('/').nth(index).unwrap_or("")
}

/// Normalized database-object key: `schema/kind/name`.
#[derive(Clone, Debug, Eq, PartialEq, Hash, Serialize, Deserialize)]
pub struct ObjectKey(String);

/// Repository-relative script path.
#[derive(Clone, Debug, Eq, PartialEq, Hash, Serialize, Deserialize)]
pub struct ScriptKey(String);

impl ObjectKey {
    /// Builds a lowercase key from raw parts.
    pub fn new(schema: &str, kind: &str, name: &str) -> Self {
        Self(format!(
            "{}/{}/{}",
            schema.to_lowercase(),
            kind.to_lowercase(),
            name.to_lowercase()
        ))
    }

    /// Builds a key from an already normalized string.
    pub fn from_normalized(value: &str) -> Self {
        Self(value.to_owned())
    }

    /// Parses `<database>/<schema>/<kind>/<name>.sql`.
    pub fn parse(path: &str) -> Option<Self> {
        let path = path.trim_end_matches(".sql");
        let parts: Vec<_> = path.split('/').collect();
        if parts.len() < 4 {
            return None;
        }
        Some(Self::new(
            parts[parts.len() - 3],
            parts[parts.len() - 2],
            parts.last()?,
        ))
    }

    /// Returns `schema/kind/name`.
    pub fn as_str(&self) -> &str {
        &self.0
    }

    /// Returns the schema segment.
    pub fn schema_part(&self) -> &str {
        key_part(self.as_str(), 0)
    }

    /// Returns the kind segment.
    pub fn kind_part(&self) -> &str {
        key_part(self.as_str(), 1)
    }

    /// Returns the object-name segment.
    pub fn name_part(&self) -> &str {
        key_part(self.as_str(), 2)
    }

    /// Returns an owned schema segment.
    pub fn schema_shared(&self) -> String {
        self.schema_part().to_owned()
    }

    /// Returns an owned kind segment.
    pub fn kind_shared(&self) -> String {
        self.kind_part().to_owned()
    }

    /// Returns an owned object-name segment.
    pub fn name_shared(&self) -> String {
        self.name_part().to_owned()
    }

    /// Clones the normalized key string.
    pub fn shared(&self) -> String {
        self.0.clone()
    }
}

impl fmt::Display for ObjectKey {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(&self.0)
    }
}

impl Borrow<str> for ObjectKey {
    fn borrow(&self) -> &str {
        self.as_str()
    }
}

impl ScriptKey {
    /// Builds a script key and normalizes path separators.
    pub fn from_path(path: &str) -> Self {
        Self(path.replace('\\', "/"))
    }

    /// Returns the repository-relative path.
    pub fn as_str(&self) -> &str {
        &self.0
    }

    /// Clones the repository-relative path.
    pub fn shared(&self) -> String {
        self.0.clone()
    }
}

impl From<String> for ObjectKey {
    fn from(value: String) -> Self {
        Self(value)
    }
}

impl From<String> for ScriptKey {
    fn from(value: String) -> Self {
        Self(value)
    }
}
