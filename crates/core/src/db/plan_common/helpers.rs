use std::collections::HashMap;

use crate::db::catalog_inspect_cache;
use crate::db::state::{CatalogObject, CatalogState};
use crate::domain::{ObjectKey, Workspace};
use crate::error::Result;
use crate::gate::{expand_delta_closure, keys_for_changed_paths};
use crate::plan::scope::InspectScope;
use crate::sql;

use super::conn::PlanDbConn;

pub(crate) async fn try_fast_empty_catalog(
    conn: &mut PlanDbConn<'_>,
    scope_json: &str,
) -> Result<Option<CatalogState>> {
    if scope_json == "[]" {
        return Ok(Some(CatalogState::default()));
    }
    let rows = conn.query(sql::catalog::SCOPED_HIT, &[scope_json]).await?;
    if rows.is_empty() {
        return Ok(Some(CatalogState::default()));
    }
    Ok(None)
}

pub(crate) fn merge_stable_catalog(
    state: &mut CatalogState,
    stable: &HashMap<ObjectKey, CatalogObject>,
) {
    for (k, o) in stable {
        state.objects.entry(k.clone()).or_insert_with(|| o.clone());
    }
}

pub(crate) fn schemas_json(ws: &Workspace) -> String {
    let schemas: Vec<String> = ws
        .schemas
        .iter()
        .map(|s| s.normalized.as_ref().to_string())
        .collect();
    serde_json::to_string(&schemas).unwrap_or_else(|_| "[]".into())
}

pub(crate) fn kinds_for_git_delta<'a>(ws: &'a Workspace, changed_paths: &[String]) -> Vec<&'a str> {
    let delta = expand_delta_closure(ws, keys_for_changed_paths(ws, changed_paths));
    let mut kinds = Vec::new();
    for (i, o) in ws.object_entries.iter().enumerate() {
        // Delta keys are database-qualified; workspace keys are not.
        let qualified = format!("{}/{}", ws.database_name(o.db_id), o.key_str(ws, i));
        if delta.contains(&qualified) {
            kinds.push(o.kind_part(ws, i));
        }
    }
    kinds
}

pub(crate) fn kinds_for_scope<'a>(ws: &'a Workspace, scope: &InspectScope) -> Vec<&'a str> {
    let mut kinds = Vec::new();
    for (i, o) in ws.object_entries.iter().enumerate() {
        if scope.full_inspect || scope.hot_keys.contains(o.key_str(ws, i)) {
            kinds.push(o.kind_part(ws, i));
        }
    }
    kinds
}

fn hot_keys_have_history(scope: &InspectScope, checksums: &crate::db::ChecksumMap) -> bool {
    scope.hot_keys.iter().any(|k| {
        let key = ObjectKey::from_normalized(k);
        checksums.get_key(&key).is_some_and(|cs| cs != &[0; 32])
    })
}

pub(crate) async fn should_query_catalog(
    full: bool,
    scope: &InspectScope,
    scope_json: &str,
    checksums: &crate::db::ChecksumMap,
) -> Result<bool> {
    if scope_json == "[]" {
        return Ok(false);
    }
    if full {
        return Ok(true);
    }
    if hot_keys_have_history(scope, checksums) {
        return Ok(true);
    }
    if !full && !scope.hot_keys.is_empty() {
        return Ok(true);
    }
    Ok(false)
}

pub(crate) fn store_inspect_cache(
    db_fp: &str,
    layout_digest: &[u8; 32],
    scope_json: &str,
    loaded: &CatalogState,
) {
    if loaded.objects.is_empty() && loaded.schemas.is_empty() {
        return;
    }
    catalog_inspect_cache::store(db_fp, layout_digest, scope_json, loaded);
}
