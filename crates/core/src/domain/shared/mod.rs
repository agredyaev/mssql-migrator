//! `SharedStr` wire boundary:
//! - Hot (post `intern_workspace_strings`): arena [`SharedStrInner::Slice`] via `StrOff` + `Workspace::shared_at`.
//! - Staging (scan ingest, JSON deserialize, tests): [`SharedStr::new`] / [`share`] → `Owned`.
//! - Export materialize: wire `PlannedObject` builds `SharedStr` only at materialize.
//!
//! Do not call [`SharedStr::new`] in per-object diff/plan loops after finalize.

use std::sync::{Arc, OnceLock};

mod subslice;
mod traits;

#[derive(Debug)]
pub(crate) enum SharedStrInner {
    Empty,
    /// Slice into the scan-finalize arena buffer.
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

impl SharedStr {
    /// Sub-slice of `base` for `part`; no new heap when `base` is an arena slice.
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

    /// Stable map key from backing bytes (slice/owned use the same rules as [`super::key_fingerprint`]).
    pub fn fingerprint(&self) -> u64 {
        super::key_fingerprint(self.hash_bytes())
    }

    /// UTF-8 bytes for fingerprinting / equality (validates arena slices once).
    pub(crate) fn hash_bytes(&self) -> &[u8] {
        match &*self.0 {
            SharedStrInner::Empty => b"",
            SharedStrInner::Slice { buf, start, len } => {
                let start = *start as usize;
                let end = start + *len as usize;
                &buf[start..end]
            }
            SharedStrInner::Owned(s) => s.as_ref().as_bytes(),
        }
    }
}

pub fn share(s: impl AsRef<str>) -> SharedStr {
    SharedStr::new(s)
}

/// Returns a canonical empty [`SharedStr`].
///
/// The `OnceLock` caches a single `SharedStrInner::Empty` allocation across
/// the process lifetime, avoiding repeated heap allocations for every
/// empty-string reference.
pub fn empty_str() -> SharedStr {
    static EMPTY: OnceLock<SharedStr> = OnceLock::new();
    EMPTY
        .get_or_init(|| SharedStr(Arc::new(SharedStrInner::Empty)))
        .clone()
}
