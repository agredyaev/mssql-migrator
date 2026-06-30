use std::collections::HashMap;

use super::StringInterner;
use crate::domain::SharedStr;

impl StringInterner {
    /// Creates a `StringInterner` with a pre-allocated index table for `unique_hint` strings.
    pub fn with_capacity(unique_hint: usize) -> Self {
        Self {
            index: HashMap::with_capacity(unique_hint),
        }
    }

    /// Interns `s`, returning a deduplicated shared handle.
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

    /// Returns the number of unique strings currently held.
    pub fn unique_count(&self) -> usize {
        self.index.len()
    }

    /// Returns the total byte length of all interned strings.
    pub fn byte_len(&self) -> usize {
        self.index.keys().map(|k| k.len()).sum()
    }
}
