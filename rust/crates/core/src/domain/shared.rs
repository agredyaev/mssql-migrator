//! `SharedStr` VIEW boundary (**A3** / **CASE-6**):
//! - **Hot** (post `intern_workspace_strings`): arena [`SharedStrInner::Slice`] via `StrOff` + `Workspace::shared_at`.
//! - **Staging** (scan ingest, JSON deserialize, tests): [`SharedStr::new`] / [`share`] → `Owned`.
//! - **Export** ([`crate::export::materialize`]): wire [`PlannedObject`] builds `SharedStr` only at materialize.
//! Do not call [`SharedStr::new`] in per-object diff/plan loops after finalize.

use std::borrow::Borrow;
use std::fmt;
use std::hash::{Hash, Hasher};
use std::sync::{Arc, OnceLock};

use serde::{Deserialize, Deserializer, Serialize, Serializer};

#[derive(Debug)]
pub(crate) enum SharedStrInner {
    Empty,
    /// Kelley-style slice into one scan-finalize arena buffer (**CASE-6**).
    Slice {
        buf: Arc<[u8]>,
        start: u32,
        len: u32,
    },
    /// Pre-intern / cold-path owned string (`share()` before scan finalize).
    Owned(Arc<str>),
}

#[derive(Clone, Debug)]
pub struct SharedStr(pub(crate) Arc<SharedStrInner>);

#[path = "shared_subslice.rs"]
mod subslice;

impl SharedStr {
    /// Arena sub-slice of `base` for `part` (**CASE-6**); no new heap when `base` is arena Slice.
    pub fn subslice_of(base: &Self, part: &str) -> Self {
        subslice::subslice_of(base, part)
    }

    pub fn new(s: impl AsRef<str>) -> Self {
        let s = s.as_ref();
        if s.is_empty() {
            return empty_str();
        }
        Self(Arc::new(SharedStrInner::Owned(Arc::from(s))))
    }

    pub(crate) fn from_arena_slice(buf: Arc<[u8]>, start: u32, len: u32) -> Self {
        if len == 0 {
            return empty_str();
        }
        Self(Arc::new(SharedStrInner::Slice { buf, start, len }))
    }

    pub fn empty() -> Self {
        empty_str()
    }

    pub fn is_empty(&self) -> bool {
        self.as_str().is_empty()
    }

    pub fn as_str(&self) -> &str {
        match &*self.0 {
            SharedStrInner::Empty => "",
            SharedStrInner::Slice { buf, start, len } => {
                let start = *start as usize;
                let end = start + *len as usize;
                std::str::from_utf8(&buf[start..end]).expect("arena utf-8")
            }
            SharedStrInner::Owned(s) => s,
        }
    }

    pub fn len(&self) -> usize {
        self.as_str().len()
    }

    /// Stable map key from backing bytes (no UTF-8 validation on arena slices).
    pub fn fingerprint(&self) -> u64 {
        use std::hash::{Hash, Hasher};
        let mut h = std::collections::hash_map::DefaultHasher::new();
        match &*self.0 {
            SharedStrInner::Empty => b"".hash(&mut h),
            SharedStrInner::Slice { buf, start, len } => {
                let start = *start as usize;
                let end = start + *len as usize;
                buf[start..end].hash(&mut h);
            }
            SharedStrInner::Owned(s) => s.as_ref().hash(&mut h),
        }
        h.finish()
    }
}

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
        match &*self.0 {
            SharedStrInner::Empty => "".hash(state),
            SharedStrInner::Slice { buf, start, len } => {
                let start = *start as usize;
                let end = start + *len as usize;
                buf[start..end].hash(state);
            }
            SharedStrInner::Owned(s) => s.hash(state),
        }
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

impl Borrow<str> for SharedStr {
    fn borrow(&self) -> &str {
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

pub fn share(s: impl AsRef<str>) -> SharedStr {
    SharedStr::new(s)
}

pub fn empty_str() -> SharedStr {
    static EMPTY: OnceLock<SharedStr> = OnceLock::new();
    EMPTY
        .get_or_init(|| SharedStr(Arc::new(SharedStrInner::Empty)))
        .clone()
}
