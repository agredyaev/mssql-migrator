use crate::export::MigrationPlan;
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

pub(super) fn merge_plan(dst: &mut Option<MigrationPlan>, src: MigrationPlan) {
    match dst {
        None => *dst = Some(src),
        Some(dst) => merge_plan_into(dst, src),
    }
}

fn merge_plan_into(dst: &mut MigrationPlan, mut src: MigrationPlan) {
    let row_offset = dst.rows.len() as u32;

    if dst.command.is_empty() {
        dst.command = src.command.clone();
    }
    if dst.planned_at.is_empty() {
        dst.planned_at = src.planned_at.clone();
    }

    dst.blocked |= src.blocked;
    dst.blockers.append(&mut src.blockers);
    dst.schemas.append(&mut src.schemas);
    dst.rows.append(&mut src.rows);
    dst.objects.append(&mut src.objects);

    for (idx, git) in src.plan_git.drain() {
        dst.plan_git.insert(row_offset + idx, git);
    }
    for (idx, transitions) in src.plan_transitions.drain() {
        dst.plan_transitions.insert(row_offset + idx, transitions);
    }

    dst.summary.schema_count = dst
        .summary
        .schema_count
        .saturating_add(src.summary.schema_count);
    dst.summary.object_count = dst
        .summary
        .object_count
        .saturating_add(src.summary.object_count);
    dst.summary.create_count = dst
        .summary
        .create_count
        .saturating_add(src.summary.create_count);
    dst.summary.adopt_count = dst
        .summary
        .adopt_count
        .saturating_add(src.summary.adopt_count);
    dst.summary.skip_count = dst
        .summary
        .skip_count
        .saturating_add(src.summary.skip_count);
    dst.summary.changed_count = dst
        .summary
        .changed_count
        .saturating_add(src.summary.changed_count);
    dst.summary.blocked_count = dst
        .summary
        .blocked_count
        .saturating_add(src.summary.blocked_count);
}

#[cfg(test)]
#[path = "../../tests/merge_test.rs"]
mod merge_tests;
