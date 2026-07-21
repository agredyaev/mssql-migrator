use std::time::Instant;

use crate::db::batch;
use crate::db::catalog;
use crate::db::catalog_inspect_cache;
use crate::db::state::CatalogState;
use crate::error::Result;
use crate::plan::git_hot_scope_json;
use crate::plan::scope::InspectScope;
use crate::timings;

use super::super::super::conn::PlanDbConn;
use super::super::super::helpers::{kinds_for_git_delta, kinds_for_scope, store_inspect_cache};
use super::super::super::types::RunBodyContext;
use super::warmup::GitDeltaWarmup;

pub(super) async fn load_git_delta_catalog(
    ctx: &RunBodyContext<'_>,
    conn: &mut PlanDbConn<'_>,
    warm: &mut GitDeltaWarmup,
    scope: &InspectScope,
    scope_json: &str,
    schemas_json: &str,
    mut loaded: CatalogState,
) -> Result<CatalogState> {
    if let Some(cached) = catalog_inspect_cache::try_get_unless_bypassed(
        ctx.bypass,
        ctx.db_fp,
        &ctx.ws.layout_digest,
        scope_json,
    ) {
        warm.local_trace.flags.scoped_hit = true;
        return Ok(cached);
    }
    if warm.partial_cache {
        query_partial_cache(
            ctx,
            conn,
            warm,
            scope,
            scope_json,
            schemas_json,
            &mut loaded,
        )
        .await?;
    } else {
        query_delta_paths(ctx, conn, warm, schemas_json, &mut loaded).await?;
    }
    if !warm.local_trace.flags.scoped_hit {
        store_inspect_cache(ctx.db_fp, &ctx.ws.layout_digest, scope_json, &loaded);
    }
    Ok(loaded)
}

async fn query_partial_cache(
    ctx: &RunBodyContext<'_>,
    conn: &mut PlanDbConn<'_>,
    warm: &mut GitDeltaWarmup,
    scope: &InspectScope,
    scope_json: &str,
    schemas_json: &str,
    loaded: &mut CatalogState,
) -> Result<()> {
    let kinds = kinds_for_scope(ctx.ws, scope);
    let sql = batch::plan_db_batch_sql(&kinds, false, true, true, false);
    let t_sql = Instant::now();
    let sets = conn
        .query_all(&sql, &["[]", scope_json, schemas_json])
        .await?;
    warm.round_trips += 1;
    warm.local_trace.timings.catalog_sql_ms += timings::dur_ms(t_sql.elapsed());
    merge_catalog_sets(loaded, sets)
}

async fn query_delta_paths(
    ctx: &RunBodyContext<'_>,
    conn: &mut PlanDbConn<'_>,
    warm: &mut GitDeltaWarmup,
    schemas_json: &str,
    loaded: &mut CatalogState,
) -> Result<()> {
    let hit_scope = git_hot_scope_json(ctx.ws, &ctx.git.paths);
    let kinds = kinds_for_git_delta(ctx.ws, &ctx.git.paths);
    let sql = batch::plan_db_batch_sql(&kinds, false, true, false, false);
    let t_sql = Instant::now();
    let sets = conn
        .query_all(&sql, &["[]", &hit_scope, schemas_json])
        .await?;
    warm.round_trips += 1;
    warm.local_trace.timings.catalog_sql_ms += timings::dur_ms(t_sql.elapsed());
    merge_catalog_sets(loaded, sets)
}

fn merge_catalog_sets(
    loaded: &mut CatalogState,
    sets: Vec<Vec<crate::driver::RowData>>,
) -> Result<()> {
    for set in sets {
        if catalog::looks_like_catalog_rows(&set) {
            catalog::merge_rows(loaded, &set)?;
        }
    }
    Ok(())
}
