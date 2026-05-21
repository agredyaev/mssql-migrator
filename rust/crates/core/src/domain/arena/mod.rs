use std::collections::HashMap;
use std::sync::Arc;

use super::shared::SharedStr;

mod ws_apply;
mod ws_git;
mod ws_intern;
mod ws_register;

pub use ws_git::intern_script_git_strings;
pub use ws_intern::intern_workspace_strings;

/// Kelley layout string buffer (**ARENA** / A2). Alias for the scan-finalize arena.
pub type LayoutArena = StringArena;

/// Single backing buffer for deduplicated strings (Kelley / Go `stringArena`).
pub struct StringArenaBuilder {
    buf: Vec<u8>,
    index: HashMap<Box<str>, u32>,
}

#[derive(Clone)]
pub struct StringArena {
    buf: Arc<[u8]>,
    index: HashMap<Box<str>, u32>,
}

impl std::fmt::Debug for StringArena {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("StringArena")
            .field("bytes", &self.byte_len())
            .field("unique", &self.unique_count())
            .finish()
    }
}

impl StringArenaBuilder {
    pub fn with_capacity(byte_hint: usize, unique_hint: usize) -> Self {
        Self {
            buf: Vec::with_capacity(byte_hint),
            index: HashMap::with_capacity(unique_hint),
        }
    }

    pub fn register(&mut self, s: &str) {
        if s.is_empty() || self.index.contains_key(s) {
            return;
        }
        let off = self.buf.len() as u32;
        self.buf.extend_from_slice(s.as_bytes());
        self.index.insert(s.into(), off);
    }

    pub fn finish(self) -> StringArena {
        StringArena {
            buf: Arc::from(self.buf.into_boxed_slice()),
            index: self.index,
        }
    }
}

impl StringArena {
    pub fn get(&self, s: &str) -> SharedStr {
        if s.is_empty() {
            return SharedStr::empty();
        }
        let (off, len) = self.offset_len(s);
        SharedStr::from_arena_slice(self.buf.clone(), off, len)
    }

    pub fn offset_len(&self, s: &str) -> (u32, u32) {
        if s.is_empty() {
            return (0, 0);
        }
        let off = self
            .index
            .get(s)
            .copied()
            .unwrap_or_else(|| panic!("arena missing string: {s:?}"));
        (off, s.len() as u32)
    }

    pub fn slice_bytes(&self, off: u32, len: u32) -> &[u8] {
        if len == 0 {
            return b"";
        }
        let start = off as usize;
        let end = start + len as usize;
        &self.buf[start..end]
    }

    pub fn str_at(&self, off: u32, len: u32) -> &str {
        std::str::from_utf8(self.slice_bytes(off, len)).expect("arena utf-8")
    }

    pub fn shared_at(&self, off: u32, len: u32) -> SharedStr {
        if len == 0 {
            return SharedStr::empty();
        }
        SharedStr::from_arena_slice(self.buf.clone(), off, len)
    }

    pub fn unique_count(&self) -> usize {
        self.index.len()
    }

    pub fn byte_len(&self) -> usize {
        self.buf.len()
    }
}

/// Arc-dedup interner for synthetic benches (pre-finalize); scan uses [`StringArena`].
pub struct StringInterner {
    index: HashMap<Box<str>, SharedStr>,
}

impl StringInterner {
    pub fn with_capacity(unique_hint: usize) -> Self {
        Self {
            index: HashMap::with_capacity(unique_hint),
        }
    }

    pub fn intern(&mut self, s: &str) -> SharedStr {
        if s.is_empty() {
            return SharedStr::empty();
        }
        if let Some(existing) = self.index.get(s) {
            return existing.clone();
        }
        let shared = SharedStr::new(s);
        self.index.insert(s.into(), shared.clone());
        shared
    }

    pub fn unique_count(&self) -> usize {
        self.index.len()
    }

    pub fn byte_len(&self) -> usize {
        self.index.keys().map(|k| k.len()).sum()
    }
}
