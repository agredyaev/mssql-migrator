mod checksums;
mod full;
mod incremental;

use std::time::Instant;

use crate::db::catalog_inspect_cache;
use crate::db::intern_catalog_state;
use crate::db::plan_db_trace::PlanDbTrace;
use crate::db::state::{CatalogState, ChecksumMap};
use crate::error::Result;
use crate::plan::scope::build_scope_and_json;
use crate::timings;

use crate::driver::TimingConn;

use super::super::helpers::{merge_stable_catalog, schemas_json, should_query_catalog};
use super::super::types::{BodyOutput, RunBodyContext};

use checksums::load_standard_checksums;
use full::load_full_catalog;
use incremental::{load_incremental_catalog, IncrementalCatalogParams};

pub(super) async fn run_standard_body(
    ctx: &mut RunBodyContext<'_>,
    conn: &mut TimingConn,
) -> Result<BodyOutput> {
    let mut checksums = ChecksumMap::new();
    let mut checksums_ms = 0i64;
    let mut inspect_ms = 0i64;
    let mut ensure_ms = 0i64;
    let mut round_trips = ctx.round_trips_start;
    let mut local_trace = PlanDbTrace::default();
    let mut catalog_base = ctx.catalog_base.take();

    if ctx.need_checksums {
        (checksums, checksums_ms) =
            load_standard_checksums(ctx, conn, &mut round_trips, &mut local_trace).await?;
    }

    if ctx.need_catalog {
        let t_insp = Instant::now();
        // Mutating commands (bypass) must live-check every managed object:
        // synthesized presence + a fixed spot-check sample can miss the same
        // externally dropped object forever.
        let (scope, scope_json) =
            build_scope_and_json(ctx.ws, &ctx.git.paths, ctx.full || ctx.bypass, &checksums);
        let schemas_json = schemas_json(ctx.ws);
        let query_catalog = should_query_catalog(ctx.full, &scope, &scope_json);
        local_trace.flags.catalog_queried = query_catalog;

        let mut catalog_sql_ms = 0i64;
        let mut loaded = CatalogState::default();
        if query_catalog {
            if let Some(cached) = catalog_inspect_cache::try_get_unless_bypassed(
                ctx.bypass,
                ctx.db_fp,
                &ctx.ws.layout_digest,
                &scope_json,
            ) {
                loaded = cached;
                local_trace.flags.scoped_hit = true;
            } else if ctx.full {
                loaded = load_full_catalog(
                    ctx,
                    conn,
                    &scope,
                    &scope_json,
                    &schemas_json,
                    &mut round_trips,
                    &mut catalog_sql_ms,
                )
                .await?;
            } else {
                loaded = load_incremental_catalog(
                    conn,
                    &mut IncrementalCatalogParams {
                        ctx,
                        scope: &scope,
                        scope_json: &scope_json,
                        schemas_json: &schemas_json,
                        local_trace: &mut local_trace,
                        round_trips: &mut round_trips,
                        catalog_sql_ms: &mut catalog_sql_ms,
                        ensure_ms: &mut ensure_ms,
                    },
                )
                .await?;
            }
        }
        local_trace.timings.catalog_sql_ms = catalog_sql_ms;
        merge_stable_catalog(&mut loaded, &scope.stable_objects);
        let t_intern = Instant::now();
        intern_catalog_state(&mut loaded);
        local_trace.timings.intern_catalog_ms += timings::dur_ms(t_intern.elapsed());
        inspect_ms = timings::dur_ms(t_insp.elapsed());
        local_trace.timings.catalog_ms =
            local_trace.timings.catalog_sql_ms + local_trace.timings.intern_catalog_ms;
        catalog_base = Some(loaded);
    }

    local_trace.timings.round_trips = round_trips;
    Ok(BodyOutput {
        checksums,
        catalog: catalog_base.unwrap_or_default(),
        checksums_ms,
        inspect_ms,
        ensure_ms,
        trace: local_trace,
    })
}
