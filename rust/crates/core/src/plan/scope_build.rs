use std::collections::{HashMap, HashSet};

use crate::db::state::{catalog_object_parts, CatalogObject, ChecksumMap};
use crate::domain::{ObjectKey, Workspace};
use crate::gate::{expand_delta_closure, keys_for_changed_paths};

use super::scope::InspectScope;

pub fn build_inspect_scope(
    ws: &Workspace,
    changed_paths: &[String],
    full_inspect: bool,
    checksums: &ChecksumMap,
) -> InspectScope {
    if full_inspect {
        return InspectScope {
            full_inspect: true,
            hot_keys: ws.normalized_keys().into_iter().collect(),
            stable_objects: HashMap::new(),
            allow_l1_skip: false,
        };
    }
    let mut delta = keys_for_changed_paths(ws, changed_paths);
    delta = expand_delta_closure(ws, delta);
    let total = ws.object_count();
    let mut hot: HashSet<String> = HashSet::new();
    let mut stable: HashMap<ObjectKey, CatalogObject> = HashMap::new();
    for i in 0..total {
        let obj = ws.entry(i);
        let key = ws.entry_key(i);
        let k = key.as_str();
        if delta.contains(k) {
            hot.insert(k.to_string());
            continue;
        }
        let file_cs = obj.checksum;
        let prior = checksums.get_key(key);
        if prior.is_none() || prior == Some(&[0; 32]) || prior != Some(&file_cs) {
            hot.insert(k.to_string());
            continue;
        }
        stable.insert(
            key.clone(),
            catalog_object_parts(
                obj.schema_shared(ws),
                obj.kind_shared(ws),
                obj.name_shared(ws),
                obj.parent_ref_for_row(ws, ws.row_id_at(i))
                    .filter(|p| p.parent_row_id > 0)
                    .map(|_| obj.parent_name(ws, ws.row_id_at(i))),
            ),
        );
    }
    promote_spot_check_keys(&mut hot, &mut stable, ws);
    if hot.len() == total {
        return InspectScope {
            full_inspect: true,
            hot_keys: hot,
            stable_objects: HashMap::new(),
            allow_l1_skip: false,
        };
    }
    let allow_l1_skip = hot.is_empty();
    InspectScope {
        full_inspect: false,
        hot_keys: hot,
        stable_objects: stable,
        allow_l1_skip,
    }
}

fn promote_spot_check_keys(
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
    let take = n.min(keys.len());
    for key in keys.into_iter().take(take) {
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
    let sum = h.finalize();
    u64::from_le_bytes(sum[..8].try_into().expect("8 bytes"))
}
