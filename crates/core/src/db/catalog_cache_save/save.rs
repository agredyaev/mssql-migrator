use crate::db::catalog_cache::missing_catalog_table;
use crate::db::state::CatalogState;
use crate::domain::Workspace;
use crate::driver::TimingConn;
use crate::error::Result;

use super::marshal::{filter_for_layout, marshal_rows};

// One transaction: concurrent unlocked savers must never interleave rows from
// one layout with metadata from another (torn cache).
const CACHE_SAVE_BATCH: &str = concat!(
    "SET XACT_ABORT ON;\nBEGIN TRANSACTION;\n",
    include_str!("../../../../../sql/catalog/catalog_cache_delete_all.sql"),
    "\n",
    include_str!("../../../../../sql/catalog/catalog_cache_insert_openjson.sql"),
    "\n",
    // MERGE uses @p2 digest + @p3 count; INSERT uses @p1 payload + @p2 digest.
    include_str!("../../../../../sql/catalog/catalog_meta_merge.sql"),
    "COMMIT TRANSACTION;"
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

#[cfg(test)]
#[path = "../../tests/cache_save_batch_test.rs"]
mod cache_save_batch_tests;
