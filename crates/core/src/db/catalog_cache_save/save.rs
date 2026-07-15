use crate::db::catalog_cache::missing_catalog_table;
use crate::db::state::CatalogState;
use crate::domain::Workspace;
use crate::driver::TimingConn;
use crate::error::Result;

use super::marshal::{filter_for_layout, marshal_rows};

const CACHE_SAVE_BATCH: &str = concat!(
    include_str!("../../../../../sql/catalog/catalog_cache_delete_all.sql"),
    "\n",
    include_str!("../../../../../sql/catalog/catalog_cache_insert_openjson.sql"),
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

/// Persist catalog rows in one TDS round-trip (DELETE + INSERT + meta MERGE).
pub async fn save_batched(
    conn: &mut TimingConn,
    layout_digest: &[u8; 32],
    ws: &Workspace,
    state: &CatalogState,
) -> Result<()> {
    let object_count = ws.object_count();
    if object_count == 0 {
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

/// Persists the catalog state to the cache in a single TDS round-trip.
pub async fn save(
    conn: &mut TimingConn,
    layout_digest: &[u8; 32],
    ws: &Workspace,
    state: &CatalogState,
) -> Result<()> {
    save_batched(conn, layout_digest, ws, state).await
}
