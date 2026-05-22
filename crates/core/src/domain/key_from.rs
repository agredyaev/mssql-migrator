use crate::domain::shared::{share, SharedStr};

use super::{ObjectKey, ScriptKey};

impl From<SharedStr> for ObjectKey {
    fn from(s: SharedStr) -> Self {
        Self(s)
    }
}

impl From<SharedStr> for ScriptKey {
    fn from(s: SharedStr) -> Self {
        Self(s)
    }
}

impl From<String> for ObjectKey {
    fn from(s: String) -> Self {
        Self(share(s))
    }
}

impl From<String> for ScriptKey {
    fn from(s: String) -> Self {
        Self(share(s))
    }
}
