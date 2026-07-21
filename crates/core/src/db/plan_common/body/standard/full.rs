use std::time::Instant;

use crate::db::batch;
use crate::db::catalog;
use crate::db::state::CatalogState;
use crate::error::Result;
use crate::plan::scope::InspectScope;
use crate::timings;

use crate::driver::TimingConn;

use super::super::super::helpers::{kinds_for_scope, store_inspect_cache, try_fast_empty_catalog};
use super::super::super::types::RunBodyContext;

pub(super) async fn load_full_catalog(
    ctx: &RunBodyContext<'_>,
    conn: &mut TimingConn,
    scope: &InspectScope,
    scope_json: &str,
    schemas_json: &str,
    round_trips: &mut i64,
    catalog_sql_ms: &mut i64,
) -> Result<CatalogState> {
    let t_sql = Instant::now();
    let kinds = kinds_for_scope(ctx.ws, scope);
    if fast_empty_provable(&kinds) {
        if let Some(empty) = try_fast_empty_catalog(conn, scope_json).await? {
            *round_trips += 1;
            *catalog_sql_ms += timings::dur_ms(t_sql.elapsed());
            return Ok(empty);
        }
    }
    let sql = batch::plan_db_batch_sql(&kinds, false, true, false, false);
    let sets = conn
        .query_all(&sql, &["[]", scope_json, schemas_json])
        .await?;
    *round_trips += 1;
    *catalog_sql_ms += timings::dur_ms(t_sql.elapsed());
    let mut loaded = CatalogState::default();
    for set in sets {
        if catalog::looks_like_catalog_rows(&set) {
            catalog::merge_rows(&mut loaded, &set)?;
        }
    }
    store_inspect_cache(ctx.db_fp, &ctx.ws.layout_digest, scope_json, &loaded);
    Ok(loaded)
}

// The fast-empty probe reads sys.objects only; indexes and table types
// live in sys.indexes / sys.table_types, so zero sys.objects hits cannot
// prove such a scope empty.
fn fast_empty_provable(kinds: &[&str]) -> bool {
    !kinds.iter().any(|k| *k == "indexes" || *k == "types")
}

#[cfg(test)]
#[path = "../../../../tests/fast_empty_test.rs"]
mod fast_empty_tests;
