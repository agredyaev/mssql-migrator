use crate::db::plan_db_trace::PlanDbTrace;

pub(crate) fn merge_trace(dst: &mut PlanDbTrace, src: &PlanDbTrace) {
    if src.timings.checksums_batch_ms > 0 {
        dst.timings.checksums_batch_ms = src.timings.checksums_batch_ms;
    }
    if src.timings.catalog_sql_ms > 0 {
        dst.timings.catalog_sql_ms = src.timings.catalog_sql_ms;
    }
    if src.timings.catalog_ms > 0 {
        dst.timings.catalog_ms = src.timings.catalog_ms;
    }
    dst.flags.scoped_hit |= src.flags.scoped_hit;
    dst.flags.catalog_queried |= src.flags.catalog_queried;
    dst.flags.history_empty |= src.flags.history_empty;
    dst.flags.checksums_skipped |= src.flags.checksums_skipped;
    if src.timings.round_trips > dst.timings.round_trips {
        dst.timings.round_trips = src.timings.round_trips;
    }
}
