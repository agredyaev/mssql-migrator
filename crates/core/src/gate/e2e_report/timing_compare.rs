use super::types::E2EWorkflowTimings;

pub(super) fn compare_workflow_timings(
    baseline: &E2EWorkflowTimings,
    actual: &E2EWorkflowTimings,
) -> Vec<String> {
    const SETUP_APPLY_MAX_MS: i64 = 500;
    let (factor, slack_ms) = super::timing_env();
    // Scale the hard cap by the same factor/slack as relative ceilings, so a
    // loaded shared runner does not fail on an absolute-time blip.
    let setup_apply_max = (SETUP_APPLY_MAX_MS as f64 * factor).ceil() as i64 + slack_ms;

    let phases: &[(&str, i64, i64)] = &[
        (
            "setup_apply_ms",
            baseline.setup_apply_ms,
            actual.setup_apply_ms,
        ),
        (
            "plan_parallel_wall_ms",
            baseline.plan_parallel_wall_ms,
            actual.plan_parallel_wall_ms,
        ),
        ("plan_wall_ms", baseline.plan_wall_ms, actual.plan_wall_ms),
        (
            "migrate_parallel_wall_ms",
            baseline.migrate_parallel_wall_ms,
            actual.migrate_parallel_wall_ms,
        ),
        (
            "migrate_wall_ms",
            baseline.migrate_wall_ms,
            actual.migrate_wall_ms,
        ),
        ("total_ms", baseline.total_ms, actual.total_ms),
    ];

    let mut msgs = Vec::new();
    for (name, baseline_ms, actual_ms) in phases {
        if *baseline_ms == 0 && *actual_ms == 0 {
            continue;
        }
        if *baseline_ms > 0 && *actual_ms == 0 {
            msgs.push(format!(
                "{name}: baseline={baseline_ms}ms actual=0ms (missing measurement)"
            ));
            continue;
        }
        if *name == "setup_apply_ms" && *actual_ms > setup_apply_max {
            msgs.push(format!(
                "setup_apply_ms: actual={actual_ms}ms exceeds hard max {setup_apply_max}ms (warm baseline required; run apply_smoke_setup or e2e-all order)"
            ));
            continue;
        }
        super::push_ceiling(&mut msgs, name, *baseline_ms, *actual_ms);
    }
    if !baseline.plan_db_path.is_empty() && baseline.plan_db_path != actual.plan_db_path {
        msgs.push(format!(
            "plan_db_path: baseline={} actual={}",
            baseline.plan_db_path, actual.plan_db_path
        ));
    }
    msgs
}
