use crate::db::state::{catalog_object, CatalogState};
use crate::domain::ObjectKey;
use crate::driver::TimingConn;
use crate::error::{Error, Result};
use crate::sql;

/// Attempts to load cached catalog state from the DB; returns `None` on miss.
pub async fn try_load(
    conn: &mut TimingConn,
    layout_digest: &[u8; 32],
    object_count: usize,
) -> Result<Option<CatalogState>> {
    let digest_hex = hex::encode(layout_digest);
    let count = object_count.to_string();
    let rows = match conn
        .query(
            sql::catalog::CACHE_LOAD,
            &[digest_hex.as_str(), count.as_str()],
        )
        .await
    {
        Ok(rows) => rows,
        Err(e) if missing_catalog_table(&e) => return Ok(None),
        Err(e) => return Err(e),
    };
    if rows.is_empty() {
        return Ok(None);
    }
    let mut state = CatalogState::default();
    for row in rows {
        merge_row(&mut state, &row)?;
    }
    if state.objects.len() != object_count {
        return Ok(None);
    }
    hydrate_schemas_from_objects(&mut state);
    crate::db::intern_catalog_state(&mut state);
    Ok(Some(state))
}

/// Load cached catalog when object count matches meta (git delta: digest may differ).
pub fn merge_load_rows(state: &mut CatalogState, rows: &[crate::driver::RowData]) -> Result<()> {
    for row in rows {
        merge_row(state, row)?;
    }
    hydrate_schemas_from_objects(state);
    Ok(())
}

fn hydrate_schemas_from_objects(state: &mut CatalogState) {
    for o in state.objects.values() {
        state.schemas.insert(o.schema.as_ref().to_lowercase());
    }
}

fn merge_row(state: &mut CatalogState, row: &crate::driver::RowData) -> Result<()> {
    let key = row.get_str(0).unwrap_or("");
    let schema = row.get_str(1).unwrap_or("");
    let kind = row.get_str(2).unwrap_or("");
    let name = row.get_str(3).unwrap_or("");
    let parent = row.get_str(4);
    if kind == "schema" {
        state.schemas.insert(schema.to_lowercase());
        return Ok(());
    }
    state.objects.insert(
        ObjectKey::from_normalized(key),
        catalog_object(schema, kind, name, parent),
    );
    Ok(())
}

/// Marks the catalog cache stale when `enabled` (`cfg.catalog_cache()`).
pub async fn invalidate(conn: &mut TimingConn, enabled: bool) -> Result<()> {
    if !enabled {
        return Ok(());
    }
    if let Err(e) = conn.exec(sql::catalog::CACHE_INVALIDATE).await {
        if missing_catalog_table(&e) {
            return Ok(());
        }
        return Err(e);
    }
    Ok(())
}

pub(crate) fn missing_catalog_table(err: &Error) -> bool {
    let Error::Sql(msg) = err else {
        return false;
    };
    let m = msg.to_lowercase();
    m.contains("catalog_cache") || m.contains("catalog_meta") || m.contains("invalid object name")
}
