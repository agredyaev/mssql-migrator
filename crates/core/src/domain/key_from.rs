use crate::domain::shared::SharedStr;

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
