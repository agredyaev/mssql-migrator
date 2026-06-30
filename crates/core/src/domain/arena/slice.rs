use std::sync::Arc;

use super::StringArena;
use crate::domain::shared::{SharedStr, SharedStrInner};
use crate::domain::str_off::StrOff;

impl StringArena {
    /// Returns the interned string for `s` as a `SharedStr`.
    pub fn get(&self, s: &str) -> SharedStr {
        if s.is_empty() {
            return SharedStr::empty();
        }
        let (off, len) = self.offset_len(s);
        SharedStr::from_arena_slice(self.buf.clone(), off, len)
    }

    /// Returns the `(offset, length)` pair locating `s` within the arena buffer.
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

    /// Returns the raw byte slice at `(off, len)` in the arena buffer.
    pub fn slice_bytes(&self, off: u32, len: u32) -> &[u8] {
        if len == 0 {
            return b"";
        }
        let start = off as usize;
        let end = start + len as usize;
        &self.buf[start..end]
    }

    /// Returns the UTF-8 string at `(off, len)` in the arena buffer.
    pub fn str_at(&self, off: u32, len: u32) -> &str {
        match std::str::from_utf8(self.slice_bytes(off, len)) {
            Ok(s) => s,
            Err(err) => panic!("arena UTF-8 invariant violated: {err}"),
        }
    }

    /// Returns a `SharedStr` backed by the arena slice at `(off, len)`.
    pub fn shared_at(&self, off: u32, len: u32) -> SharedStr {
        if len == 0 {
            return SharedStr::empty();
        }
        SharedStr::from_arena_slice(self.buf.clone(), off, len)
    }

    /// Returns the number of unique strings interned in the arena.
    pub fn unique_count(&self) -> usize {
        self.index.len()
    }

    /// Returns the total byte length of the arena buffer.
    pub fn byte_len(&self) -> usize {
        self.buf.len()
    }

    /// If `s` already slices this arena buffer, return `StrOff` without hash lookup or `String` clone.
    pub fn str_off_if_dedicated(&self, s: &SharedStr) -> Option<StrOff> {
        match &*s.0 {
            SharedStrInner::Slice { buf, start, len } if Arc::ptr_eq(buf, &self.buf) => {
                Some(StrOff(*start, *len))
            }
            _ => None,
        }
    }
}
