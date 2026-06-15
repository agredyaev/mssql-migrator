use std::time::Instant;

use crate::db::plan_db_trace::PlanDbTrace;
use crate::db::state::ChecksumMap;
use crate::error::Result;
use crate::timings;

use super::super::super::checksums::{
    checksum_query_round_trips, ensure_tables_plan, load_checksums_plan, set_checksum_trace,
};
use super::super::super::conn::PlanDbConn;
use super::super::super::types::RunBodyContext;

pub(super) async fn load_standard_checksums(
    ctx: &RunBodyContext<'_>,
    conn: &mut PlanDbConn<'_>,
    round_trips: &mut i64,
    local_trace: &mut PlanDbTrace,
) -> Result<(ChecksumMap, i64)> {
    let t_cs = Instant::now();
    if ctx.bootstrap_in_sql && !crate::audit::tables_ensured(ctx.db_fp) {
        ensure_tables_plan(conn, ctx.db_fp).await?;
    }
    let checksums = load_checksums_plan(conn, ctx.db_fp, ctx.keys_json).await?;
    let checksums_ms = timings::dur_ms(t_cs.elapsed());
    set_checksum_trace(local_trace, ctx.db_fp, ctx.keys_json);
    *round_trips += checksum_query_round_trips(ctx.db_fp, ctx.keys_json);
    local_trace.timings.checksums_batch_ms = checksums_ms;
    Ok((checksums, checksums_ms))
}
