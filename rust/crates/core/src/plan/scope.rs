use serde_json::json;

use std::collections::{HashMap, HashSet};

use crate::db::state::ChecksumMap;
use crate::db::state::CatalogObject;
use crate::domain::Workspace;

pub use super::scope_build::build_inspect_scope;

#[derive(Clone, Debug)]
pub struct InspectScope {
    pub full_inspect: bool,
    pub hot_keys: HashSet<String>,
    /// Objects with file digest == audit history; merged into catalog without SQL lookup (Go parity).
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

pub fn apply_checksums_if_needed(
    ws: &mut Workspace,
    checksums: &ChecksumMap,
) {
    if ws.checksums_applied() {
        return;
    }
    apply_checksums(ws, checksums);
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
    ws.for_each_entry_mut(|obj| {
        obj.db.exists = catalog.exists_key(&obj.key);
        if let Some(cat) = catalog.objects.get(&obj.key) {
            if let Some(parent) = &cat.parent {
                obj.parent_name = parent.clone();
                obj.parent_key = Some(crate::domain::ObjectKey::new(
                    obj.schema.as_ref(),
                    "tables",
                    parent.as_ref(),
                ));
            }
        }
    });
}

pub fn apply_checksums(
    ws: &mut Workspace,
    checksums: &ChecksumMap,
) {
    ws.for_each_entry_mut(|obj| {
        if let Some(cs) = checksums.get(&obj.key) {
            obj.history = Some(*cs);
        }
    });
}
