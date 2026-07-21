use std::time::Instant;

use crate::audit;
use crate::db::batch;
use crate::db::catalog;
use crate::db::state::CatalogState;
use crate::error::Result;
use crate::plan::scope::InspectScope;
use crate::timings;

use crate::driver::TimingConn;

use super::super::super::checksums::ensure_tables_plan;
use super::super::super::helpers::{kinds_for_scope, store_inspect_cache};
use super::super::super::types::RunBodyContext;
use crate::db::plan_db_trace::PlanDbTrace;

pub(super) struct IncrementalCatalogParams<'a> {
    pub ctx: &'a RunBodyContext<'a>,
    pub scope: &'a InspectScope,
    pub scope_json: &'a str,
    pub schemas_json: &'a str,
    pub local_trace: &'a mut PlanDbTrace,
    pub round_trips: &'a mut i64,
    pub catalog_sql_ms: &'a mut i64,
    pub ensure_ms: &'a mut i64,
}

pub(super) async fn load_incremental_catalog(
    conn: &mut TimingConn,
    p: &mut IncrementalCatalogParams<'_>,
) -> Result<CatalogState> {
    let single_rt = p.ctx.bootstrap_in_sql && p.local_trace.flags.checksums_skipped;
    if p.ctx.bootstrap_in_sql && !single_rt && !audit::tables_ensured(p.ctx.db_fp) {
        let t0 = Instant::now();
        ensure_tables_plan(conn, p.ctx.db_fp).await?;
        *p.ensure_ms = timings::dur_ms(t0.elapsed());
    }
    let kinds = kinds_for_scope(p.ctx.ws, p.scope);
    let sql = if single_rt {
        batch::plan_db_batch_sql(&kinds, true, true, false, false)
    } else {
        batch::plan_db_batch_sql(&kinds, false, true, false, false)
    };
    let t_sql = Instant::now();
    let sets = conn
        .query_all(&sql, &["[]", p.scope_json, p.schemas_json])
        .await?;
    *p.round_trips += 1;
    *p.catalog_sql_ms += timings::dur_ms(t_sql.elapsed());
    if single_rt {
        audit::mark_tables_ensured(p.ctx.db_fp);
    }
    let mut loaded = CatalogState::default();
    for set in sets {
        if catalog::looks_like_catalog_rows(&set) {
            catalog::merge_rows(&mut loaded, &set)?;
        }
    }
    store_inspect_cache(p.ctx.db_fp, &p.ctx.ws.layout_digest, p.scope_json, &loaded);
    Ok(loaded)
}
