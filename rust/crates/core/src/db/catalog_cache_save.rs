use serde::Serialize;

use super::catalog_cache::{cache_enabled, missing_catalog_table};
use super::state::{catalog_object_parts, CatalogState};
use crate::domain::Workspace;
use crate::driver::TimingConn;
use crate::error::Result;

const CACHE_SAVE_BATCH: &str = concat!(
    include_str!("../../../../sql/catalog/catalog_cache_delete_all.sql"),
    "\n",
    include_str!("../../../../sql/catalog/catalog_cache_insert_openjson.sql"),
    "\n",
    // MERGE uses @p2 digest + @p3 count (INSERT uses @p1 payload + @p2 digest).
    "MERGE azdo_deploy_meta.catalog_meta AS t\n",
    "USING (SELECT 1 AS id) AS s ON t.id = s.id\n",
    "WHEN MATCHED THEN\n",
    "    UPDATE SET layout_digest = @p2, object_count = @p3, captured_at = SYSUTCDATETIME()\n",
    "WHEN NOT MATCHED THEN\n",
    "    INSERT (id, layout_digest, object_count, captured_at)\n",
    "    VALUES (1, @p2, @p3, SYSUTCDATETIME());"
);

#[derive(Serialize)]
struct CacheRow<'a> {
    k: &'a str,
    s: &'a str,
    g: &'a str,
    o: &'a str,
    p: &'a str,
}

/// Persist catalog rows in one TDS round-trip (DELETE + INSERT + meta MERGE).
pub async fn save_batched(
    conn: &mut TimingConn,
    layout_digest: &[u8; 32],
    ws: &Workspace,
    state: &CatalogState,
) -> Result<()> {
    let object_count = ws.object_count();
    if !cache_enabled() || object_count == 0 {
        return Ok(());
    }
    let filtered = filter_for_layout(ws, state);
    if filtered.objects.len() != object_count {
        return Ok(());
    }
    let payload = match marshal_rows(&filtered, object_count)? {
        Some(p) => p,
        None => return Ok(()),
    };
    let digest_hex = hex::encode(layout_digest);
    let count = object_count.to_string();
    if let Err(e) = conn
        .query(
            CACHE_SAVE_BATCH,
            &[payload.as_str(), digest_hex.as_str(), count.as_str()],
        )
        .await
    {
        if missing_catalog_table(&e) {
            return Ok(());
        }
        return Err(e);
    }
    Ok(())
}

pub async fn save(
    conn: &mut TimingConn,
    layout_digest: &[u8; 32],
    ws: &Workspace,
    state: &CatalogState,
) -> Result<()> {
    save_batched(conn, layout_digest, ws, state).await
}

/// Persist workspace object metadata after a successful apply (warm catalog_cache for git delta).
pub async fn save_workspace_snapshot(
    conn: &mut TimingConn,
    layout_digest: &[u8; 32],
    ws: &Workspace,
) -> Result<()> {
    if !cache_enabled() || ws.object_count() == 0 {
        return Ok(());
    }
    let mut state = CatalogState::default();
    for i in 0..ws.object_count() {
        let obj = ws.entry(i);
        let row_id = ws.row_id_at(i);
        state.schemas.insert(obj.schema_part(ws).to_lowercase());
        state.objects.insert(
            ws.entry_key(i).clone(),
            catalog_object_parts(
                obj.schema_shared(ws),
                obj.kind_shared(ws),
                obj.name_shared(ws),
                obj.parent_ref_for_row(ws, row_id)
                    .filter(|p| p.parent_row_id > 0)
                    .map(|_| obj.parent_name(ws, row_id)),
            ),
        );
    }
    save_batched(conn, layout_digest, ws, &state).await
}

fn marshal_rows(state: &CatalogState, want: usize) -> Result<Option<String>> {
    if state.objects.len() != want {
        return Ok(None);
    }
    let rows: Vec<CacheRow<'_>> = state
        .objects
        .iter()
        .map(|(k, o)| CacheRow {
            k: k.as_str(),
            s: o.schema.as_ref(),
            g: o.kind.as_ref(),
            o: o.name.as_ref(),
            p: o.parent.as_ref().map(|s| s.as_ref()).unwrap_or(""),
        })
        .collect();
    Ok(Some(
        serde_json::to_string(&rows).map_err(|e| crate::error::Error::Other(e.into()))?,
    ))
}

fn filter_for_layout(ws: &Workspace, state: &CatalogState) -> CatalogState {
    let mut out = CatalogState::default();
    ws.for_each_entry(|obj| {
        if let Some(o) = state.objects.get(&obj.key(ws)) {
            out.objects.insert(obj.key(ws), o.clone());
        }
    });
    out
}
