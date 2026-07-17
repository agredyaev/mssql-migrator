use std::collections::HashSet;

use crate::domain::Workspace;
use crate::gate::{expand_delta_closure, keys_for_changed_paths};

use super::scope::{build_scope_json, InspectScope};

/// Git delta scope for scoped-hit probe before checksums are available.
pub fn git_hot_scope_json(ws: &Workspace, changed_paths: &[String]) -> String {
    let delta = expand_delta_closure(ws, keys_for_changed_paths(ws, changed_paths));
    // Delta keys are database-qualified; workspace keys are not.
    let hot_keys: HashSet<String> = ws
        .object_entries
        .iter()
        .enumerate()
        .filter(|(i, o)| {
            let db = ws.database_name(o.db_id);
            delta.contains(&format!("{db}/{}", o.key_str(ws, *i)))
        })
        .map(|(i, o)| o.key_str(ws, i).to_string())
        .collect();
    build_scope_json(&InspectScope {
        full_inspect: false,
        hot_keys,
        stable_objects: Default::default(),
        allow_l1_skip: false,
    })
}
