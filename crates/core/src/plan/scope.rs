//! [`InspectScope`] and catalog / checksum application helpers for the diff phase.

use serde_json::json;

use std::collections::{HashMap, HashSet};

use crate::db::state::CatalogObject;
use crate::db::state::ChecksumMap;
use crate::domain::{ObjectKey, ParentRef, Workspace};

pub use super::scope_build::{build_inspect_scope, build_scope_and_json};

/// Parameters controlling which objects are inspected during a catalog scan.
#[derive(Clone, Debug)]
pub struct InspectScope {
    /// Whether every object is inspected regardless of cache state.
    pub full_inspect: bool,
    /// Normalized `schema/kind/name` keys targeted for live DB inspection.
    pub hot_keys: HashSet<String>,
    /// Objects with file digest == audit history; merged into catalog without SQL lookup.
    pub stable_objects: HashMap<crate::domain::ObjectKey, CatalogObject>,
    /// Permits skipping the L1 cache lookup when all objects are stable.
    pub allow_l1_skip: bool,
}

/// Applies catalog state to the workspace if it has not been applied yet.
pub fn apply_catalog_if_needed(ws: &mut Workspace, catalog: &crate::db::CatalogState) {
    if ws.catalog_applied() {
        return;
    }
    apply_catalog(ws, catalog);
    ws.mark_catalog_applied();
}

/// Populates the prior-checksum column from `checksums` if not already applied.
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

/// Serializes `scope.hot_keys` to a deterministic JSON string for use as an inspect-cache key.
pub fn build_scope_json(scope: &InspectScope) -> String {
    // `hot_keys` is a `HashSet`, so iteration order is non-deterministic. This JSON
    // is used as the inspect-cache key (`db::catalog_inspect_cache`), so it must be
    // stable across runs or the same logical scope produces cache misses. Sort the
    // parsed parts before serializing to make the output order deterministic.
    let mut parts: Vec<(String, String, String)> = scope
        .hot_keys
        .iter()
        .filter_map(|k| scope_key_parts(k))
        .collect();
    parts.sort();
    let refs: Vec<_> = parts
        .into_iter()
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

/// Stamps DB-existence flags and parent references onto workspace entries from `catalog`.
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

#[cfg(test)]
#[path = "../tests/scope_test.rs"]
mod scope_tests;
