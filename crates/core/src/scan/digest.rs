//! [`layout_digest`] — SHA-256 hash of the normalized workspace object layout.

use sha2::{Digest, Sha256};

use crate::domain::Workspace;

/// Computes a SHA-256 digest over the sorted set of script paths in `ws`.
pub fn layout_digest(ws: &Workspace) -> [u8; 32] {
    let mut keys: Vec<_> = ws.scripts_iter().map(|s| s.path_str()).collect();
    keys.sort();
    let mut h = Sha256::new();
    for k in keys {
        // Length-prefix each path: back-to-back concatenation lets two
        // different path sets produce identical hash input (["a","bc"] vs
        // ["ab","c"]), aliasing unrelated layouts in every cache keyed here.
        h.update((k.len() as u64).to_le_bytes());
        h.update(k.as_bytes());
    }
    h.finalize().into()
}
