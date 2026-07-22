use std::fmt;
use std::hash::{Hash, Hasher};

use serde::{Deserialize, Deserializer, Serialize, Serializer};

use super::{empty_str, SharedStr};

impl Default for SharedStr {
    fn default() -> Self {
        empty_str()
    }
}

impl PartialEq for SharedStr {
    fn eq(&self, other: &Self) -> bool {
        self.as_str() == other.as_str()
    }
}

impl Eq for SharedStr {}

impl Hash for SharedStr {
    fn hash<H: Hasher>(&self, state: &mut H) {
        self.hash_bytes().hash(state);
    }
}

impl fmt::Display for SharedStr {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(self.as_str())
    }
}

impl AsRef<str> for SharedStr {
    fn as_ref(&self) -> &str {
        self.as_str()
    }
}

impl std::ops::Deref for SharedStr {
    type Target = str;

    fn deref(&self) -> &Self::Target {
        self.as_str()
    }
}

impl From<String> for SharedStr {
    fn from(s: String) -> Self {
        Self::new(s)
    }
}

impl From<&str> for SharedStr {
    fn from(s: &str) -> Self {
        Self::new(s)
    }
}

impl Serialize for SharedStr {
    fn serialize<S: Serializer>(&self, serializer: S) -> Result<S::Ok, S::Error> {
        self.as_str().serialize(serializer)
    }
}

impl<'de> Deserialize<'de> for SharedStr {
    fn deserialize<D: Deserializer<'de>>(deserializer: D) -> Result<Self, D::Error> {
        String::deserialize(deserializer).map(Self::new)
    }
}
