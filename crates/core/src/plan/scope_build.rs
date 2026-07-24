use std::collections::{HashMap, HashSet};

use crate::db::state::{catalog_object_parts, CatalogObject, ChecksumMap};
use crate::domain::{is_module_kind_code, kind_code, ObjectKey, Workspace};
use crate::gate::{expand_delta_closure, keys_for_changed_paths};

use super::scope::InspectScope;
use super::scope_spot_check::promote_spot_check_keys;

/// Builds an inspect scope classifying workspace objects as hot (needs re-inspection) or stable.
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
        // Delta keys are database-qualified (`db/schema/kind/name`) to match
        // snapshot identities; workspace keys carry the database separately.
        let qualified = format!("{}/{k}", ws.database_name(obj.db_id));
        if is_module_kind_code(kind_code(key.kind_part())) {
            hot.insert(k.to_string());
            continue;
        }
        if delta.contains(&qualified) {
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
                obj.key.schema_shared(),
                obj.key.kind_shared(),
                obj.key.name_shared(),
                obj.parent
                    .filter(|p| p.parent_row_id > 0)
                    .map(|_| obj.parent_name(ws)),
            ),
        );
    }
    promote_spot_check_keys(&mut hot, &mut stable, ws);
    if hot.len() == total {
        return InspectScope {
            full_inspect: true,
            hot_keys: hot,
            stable_objects: HashMap::new(),
        };
    }
    InspectScope {
        full_inspect: false,
        hot_keys: hot,
        stable_objects: stable,
    }
}

/// Builds the inspect scope together with its cache-key JSON (full scopes use
/// the complete object list as the key).
pub fn build_scope_and_json(
    ws: &Workspace,
    changed_paths: &[String],
    full_inspect: bool,
    checksums: &ChecksumMap,
) -> (InspectScope, String) {
    let scope = build_inspect_scope(ws, changed_paths, full_inspect, checksums);
    let scope_json = if full_inspect {
        ws.object_scope_json()
    } else {
        super::scope::build_scope_json(&scope)
    };
    (scope, scope_json)
}
