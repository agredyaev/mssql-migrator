//! [`layout_digest`] — SHA-256 hash of the normalized workspace object layout.

use sha2::{Digest, Sha256};

use crate::domain::Workspace;

/// Computes a SHA-256 digest over the sorted set of script paths in `ws`.
pub fn layout_digest(ws: &Workspace) -> [u8; 32] {
    let mut keys: Vec<_> = ws.scripts_iter().map(|s| s.path_str()).collect();
    keys.sort();
    let mut h = Sha256::new();
    for k in keys {
        h.update(k.as_bytes());
    }
    h.finalize().into()
}
