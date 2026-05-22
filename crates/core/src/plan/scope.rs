use serde_json::json;

use std::collections::{HashMap, HashSet};

use crate::db::state::CatalogObject;
use crate::db::state::ChecksumMap;
use crate::domain::{ObjectKey, ParentRef, Workspace};

pub use super::scope_build::build_inspect_scope;

#[derive(Clone, Debug)]
pub struct InspectScope {
    pub full_inspect: bool,
    pub hot_keys: HashSet<String>,
    /// Objects with file digest == audit history; merged into catalog without SQL lookup.
    pub stable_objects: HashMap<crate::domain::ObjectKey, CatalogObject>,
    pub allow_l1_skip: bool,
}

pub fn apply_catalog_if_needed(ws: &mut Workspace, catalog: &crate::db::CatalogState) {
    if ws.catalog_applied() {
        return;
    }
    apply_catalog(ws, catalog);
    ws.mark_catalog_applied();
}

pub fn apply_checksums_if_needed(ws: &mut Workspace, checksums: &ChecksumMap) {
    if ws.checksums_applied() {
        return;
    }
    let n = ws.object_count();
    ws.prior_by_row.resize(n, None);
    for i in 0..n {
        if let Some(cs) = checksums.get_key(ws.entry_key(i)) {
            ws.prior_by_row[i] = Some(*cs);
        }
    }
    ws.mark_checksums_applied();
}

pub fn build_scope_json(scope: &InspectScope) -> String {
    let refs: Vec<_> = scope
        .hot_keys
        .iter()
        .filter_map(|k| scope_key_parts(k))
        .map(|(schema, kind, object)| json!({"schema": schema, "kind": kind, "object": object}))
        .collect();
    serde_json::to_string(&refs).unwrap_or_else(|_| "[]".into())
}

fn scope_key_parts(key: &str) -> Option<(String, String, String)> {
    let mut parts = key.split('/');
    let schema = parts.next()?.to_string();
    let kind = parts.next()?.to_string();
    let object = parts.next()?.to_string();
    Some((schema, kind, object))
}

pub fn apply_catalog(ws: &mut Workspace, catalog: &crate::db::CatalogState) {
    let n = ws.object_count();
    ws.catalog_row.resize(n, 0);
    let mut catalog_fp: HashSet<u64> = HashSet::with_capacity(catalog.objects.len());
    for key in catalog.objects.keys() {
        catalog_fp.insert(key.fingerprint());
    }
    for i in 0..n {
        let key = ws.entry_key(i).clone();
        let row_id = ws.row_id_at(i);
        let schema = key.schema_part();
        let in_catalog = catalog_fp.contains(&key.fingerprint());
        ws.entry_mut(i).set_db_exists(in_catalog);
        if in_catalog {
            ws.catalog_row[i] = 1;
        }
        if let Some(cat) = catalog.objects.get(&key) {
            if let Some(parent) = &cat.parent {
                let parent_key = ObjectKey::new(schema, "tables", parent.as_ref());
                let parent_row_id = ws.key_index(&parent_key);
                ws.parent_by_row.insert(row_id, ParentRef { parent_row_id });
            }
        }
    }
}
