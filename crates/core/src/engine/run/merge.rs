use crate::timings::PhaseTimings;

pub(super) fn merge_timings(dst: &mut PhaseTimings, src: &PhaseTimings) {
    dst.connect_ms = dst.connect_ms.max(src.connect_ms);
    dst.ensure_ms = dst.ensure_ms.saturating_add(src.ensure_ms);
    dst.checksums_ms = dst.checksums_ms.saturating_add(src.checksums_ms);
    dst.inspect_ms = dst.inspect_ms.saturating_add(src.inspect_ms);
    dst.diff_ms = dst.diff_ms.saturating_add(src.diff_ms);
    dst.apply_ms = dst.apply_ms.saturating_add(src.apply_ms);
    dst.parallel_wall_ms = dst.parallel_wall_ms.max(src.parallel_wall_ms);
    dst.plan_db_query_ms = dst.plan_db_query_ms.saturating_add(src.plan_db_query_ms);
    dst.plan_db_query_calls = dst
        .plan_db_query_calls
        .saturating_add(src.plan_db_query_calls);
    if !src.l1_cache_hit() {
        dst.set_l1_cache_hit(false);
    }
    if !src.plan_db_path.is_empty() {
        dst.plan_db_path.clone_from(&src.plan_db_path);
    }
}
