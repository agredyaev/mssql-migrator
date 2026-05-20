use sha2::{Digest, Sha256};

use crate::domain::Workspace;

pub fn layout_digest(ws: &Workspace) -> [u8; 32] {
    let mut keys: Vec<_> = ws.scripts.keys().map(|k| k.as_str()).collect();
    keys.sort();
    let mut h = Sha256::new();
    for k in keys {
        h.update(k.as_bytes());
    }
    h.finalize().into()
}
