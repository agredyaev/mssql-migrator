use std::collections::HashMap;
use std::sync::Arc;

use super::StringArena;

impl std::fmt::Debug for StringArena {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("StringArena")
            .field("bytes", &self.byte_len())
            .field("unique", &self.unique_count())
            .finish()
    }
}

impl super::StringArenaBuilder {
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
