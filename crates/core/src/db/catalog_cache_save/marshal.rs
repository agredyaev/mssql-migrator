use serde::Serialize;

use crate::db::state::CatalogState;
use crate::domain::Workspace;
use crate::error::Result;

#[derive(Serialize)]
struct CacheRow<'a> {
    k: &'a str,
    s: &'a str,
    g: &'a str,
    o: &'a str,
    p: &'a str,
}

pub(super) fn marshal_rows(state: &CatalogState) -> Result<String> {
    let rows: Vec<CacheRow<'_>> = state
        .objects
        .iter()
        .map(|(k, o)| CacheRow {
            k: k.as_str(),
            s: o.schema.as_str(),
            g: o.kind.as_str(),
            o: o.name.as_str(),
            p: o.parent.as_deref().unwrap_or(""),
        })
        .collect();
    serde_json::to_string(&rows).map_err(|e| crate::error::Error::Other(e.into()))
}

pub(super) fn filter_for_layout(ws: &Workspace, state: &CatalogState) -> CatalogState {
    let mut out = CatalogState::default();
    ws.object_entries.iter().enumerate().for_each(|(i, _obj)| {
        if let Some(o) = state.objects.get(ws.entry_key(i)) {
            out.objects.insert(ws.entry_key(i).clone(), o.clone());
        }
    });
    out
}
