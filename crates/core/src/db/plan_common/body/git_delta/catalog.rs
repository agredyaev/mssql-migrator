use std::mem;
use std::time::Instant;

use crate::db::intern_catalog_state;
use crate::db::state::CatalogState;
use crate::domain::ObjectKey;
use crate::error::Result;
use crate::plan::scope::{build_inspect_scope, build_scope_json, InspectScope};
use crate::timings;

use crate::driver::TimingConn;

use super::super::super::helpers::{merge_stable_catalog, schemas_json, should_query_catalog};
use super::super::super::types::RunBodyContext;
use super::query::load_git_delta_catalog;
use super::warmup::GitDeltaWarmup;

pub(super) async fn query_git_delta_catalog(
    ctx: &RunBodyContext<'_>,
    conn: &mut TimingConn,
    mut warm: GitDeltaWarmup,
) -> Result<(CatalogState, i64, GitDeltaWarmup)> {
    let schemas_json = schemas_json(ctx.ws);
    let scope = build_inspect_scope(ctx.ws, &ctx.git.paths, ctx.bypass, &warm.checksums);
    let scope_json = build_scope_json(&scope);
    let query_catalog = should_query_catalog(false, &scope, &scope_json);
    warm.local_trace.flags.catalog_queried = query_catalog;

    let t_cat = Instant::now();
    let mut loaded = mem::take(&mut warm.loaded);
    if query_catalog && !cache_covers_hot(warm.partial_cache && !warm.relaxed, &scope, &loaded) {
        loaded = load_git_delta_catalog(
            ctx,
            conn,
            &mut warm,
            &scope,
            &scope_json,
            &schemas_json,
            loaded,
        )
        .await?;
    }
    merge_stable_catalog(&mut loaded, &scope.stable_objects);
    let t_intern = Instant::now();
    intern_catalog_state(&mut loaded);
    warm.local_trace.timings.intern_catalog_ms += timings::dur_ms(t_intern.elapsed());
    let inspect_ms = timings::dur_ms(t_cat.elapsed());
    warm.local_trace.timings.catalog_ms =
        warm.local_trace.timings.catalog_sql_ms + warm.local_trace.timings.intern_catalog_ms;
    warm.local_trace.timings.round_trips = warm.round_trips;
    Ok((loaded, inspect_ms, warm))
}

fn cache_covers_hot(partial_cache: bool, scope: &InspectScope, loaded: &CatalogState) -> bool {
    partial_cache
        && scope
            .hot_keys
            .iter()
            .all(|k| loaded.objects.contains_key(&ObjectKey::from_normalized(k)))
}
