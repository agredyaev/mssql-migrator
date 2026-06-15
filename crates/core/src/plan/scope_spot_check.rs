use std::collections::{HashMap, HashSet};

use crate::db::state::CatalogObject;
use crate::domain::{ObjectKey, Workspace};

pub(super) fn promote_spot_check_keys(
    hot: &mut HashSet<String>,
    stable: &mut HashMap<ObjectKey, CatalogObject>,
    ws: &Workspace,
) {
    let n = spot_check_count_from_env();
    if n == 0 || stable.is_empty() {
        return;
    }
    let digest = hex::encode(ws.layout_digest);
    let mut keys: Vec<_> = stable.keys().map(|k| k.as_str().to_string()).collect();
    keys.sort_by_key(|a| spot_check_rank(a, &digest));
    for key in keys.into_iter().take(n.min(stable.len())) {
        stable.remove(&ObjectKey::from_normalized(&key));
        hot.insert(key);
    }
}

fn spot_check_count_from_env() -> usize {
    std::env::var("RMIG_CATALOG_SPOTCHECK")
        .ok()
        .and_then(|s| s.parse().ok())
        .unwrap_or(0)
}

fn spot_check_rank(key: &str, layout_digest: &str) -> u64 {
    use sha2::{Digest, Sha256};
    let mut h = Sha256::new();
    h.update(layout_digest.as_bytes());
    h.update([0u8]);
    h.update(key.as_bytes());
    let mut bytes = [0u8; 8];
    bytes.copy_from_slice(&h.finalize()[..8]);
    u64::from_le_bytes(bytes)
}
