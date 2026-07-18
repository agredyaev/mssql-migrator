use std::time::Instant;

use crate::audit;
use crate::db::batch;
use crate::db::catalog;
use crate::db::intern_catalog_state;
use crate::db::plan_db_trace::PlanDbTrace;
use crate::db::state::{CatalogState, ChecksumMap};
use crate::error::Result;
use crate::timings;

use super::super::super::checksums::{
    checksum_query_round_trips, load_checksums_plan, set_checksum_trace,
};
use super::super::super::conn::PlanDbConn;
use super::super::super::types::RunBodyContext;

pub(super) struct GitDeltaWarmup {
    pub checksums: ChecksumMap,
    pub checksums_ms: i64,
    pub loaded: CatalogState,
    pub partial_cache: bool,
    /// Cache rows came from the count-only (digest-inexact) relaxed load.
    pub relaxed: bool,
    pub round_trips: i64,
    pub local_trace: PlanDbTrace,
}

pub(super) async fn warmup_git_delta(
    ctx: &mut RunBodyContext<'_>,
    conn: &mut PlanDbConn<'_>,
) -> Result<GitDeltaWarmup> {
    let mut checksums = ChecksumMap::new();
    let mut checksums_ms = 0i64;
    let mut round_trips = ctx.round_trips_start;
    let mut local_trace = PlanDbTrace::default();
    let want_cache = ctx.cfg.catalog_cache() && audit::tables_ensured(ctx.db_fp);
    let mut loaded = ctx.catalog_base.take().unwrap_or_default();
    let mut relaxed = false;
    let partial_cache;

    if ctx.need_checksums {
        let t_cs = Instant::now();
        checksums = load_checksums_plan(
            conn,
            ctx.db_fp,
            ctx.keys_json,
            ctx.bypass,
            ctx.allow_checksum_repair,
        )
        .await?;
        checksums_ms = timings::dur_ms(t_cs.elapsed());
        set_checksum_trace(&mut local_trace, ctx.db_fp, ctx.keys_json);
        round_trips += checksum_query_round_trips(ctx.db_fp, ctx.keys_json);
    }

    if want_cache && loaded.objects.is_empty() {
        let sql = batch::plan_db_batch_sql(
            &[],
            false,
            false,
            false,
            false,
            false,
            Some(ctx.ws.object_count()),
        );
        if !sql.trim().is_empty() {
            let sets = conn.query_all(&sql, &[ctx.keys_json, "[]", "[]"]).await?;
            round_trips += 1;
            for set in sets {
                if catalog::looks_like_cache_load_rows(&set) {
                    crate::db::catalog_cache::merge_load_rows(&mut loaded, &set)?;
                    relaxed = true;
                }
            }
        }
    }

    if loaded.objects.len() == ctx.ws.object_count() {
        partial_cache = true;
        let t_intern = Instant::now();
        intern_catalog_state(&mut loaded);
        local_trace.timings.intern_catalog_ms += timings::dur_ms(t_intern.elapsed());
    } else {
        loaded = CatalogState::default();
        partial_cache = false;
    }
    local_trace.timings.checksums_batch_ms = checksums_ms;

    Ok(GitDeltaWarmup {
        checksums,
        checksums_ms,
        loaded,
        partial_cache,
        relaxed,
        round_trips,
        local_trace,
    })
}
