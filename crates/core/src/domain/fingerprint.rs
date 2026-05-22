//! Stable `u64` fingerprints for normalized object path bytes (UTF-8).

/// Stable `u64` key for normalized object paths (UTF-8 bytes).
#[inline]
pub fn key_fingerprint(bytes: &[u8]) -> u64 {
    use std::hash::{Hash, Hasher};
    let mut h = std::collections::hash_map::DefaultHasher::new();
    bytes.hash(&mut h);
    h.finish()
}
