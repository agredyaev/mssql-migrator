use crate::timings::PhaseTimings;

use super::types::E2EWorkflowTimings;

pub(super) fn compare_workflow_timings(
    baseline: &E2EWorkflowTimings,
    actual: &E2EWorkflowTimings,
) -> Vec<String> {
    const SETUP_APPLY_MAX_MS: i64 = 500;
    let factor = std::env::var("RMIG_E2E_TIMING_FACTOR")
        .ok()
        .and_then(|s| s.parse::<f64>().ok())
        .unwrap_or(3.0);
    let slack_ms: i64 = std::env::var("RMIG_E2E_TIMING_SLACK_MS")
        .ok()
        .and_then(|s| s.parse().ok())
        .unwrap_or(100);

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
        if *name == "setup_apply_ms" && *actual_ms > SETUP_APPLY_MAX_MS {
            msgs.push(format!(
                "setup_apply_ms: actual={actual_ms}ms exceeds hard max {SETUP_APPLY_MAX_MS}ms (warm baseline required; run apply_smoke_setup or e2e-all order)"
            ));
            continue;
        }
        let ceiling = ((*baseline_ms as f64) * factor).ceil() as i64 + slack_ms;
        if *actual_ms > ceiling {
            msgs.push(format!(
                "{name}: actual={actual_ms}ms > baseline={baseline_ms}ms ceiling={ceiling}ms (factor={factor}, slack={slack_ms}ms)"
            ));
        }
    }
    if !baseline.plan_db_path.is_empty() && baseline.plan_db_path != actual.plan_db_path {
        msgs.push(format!(
            "plan_db_path: baseline={} actual={}",
            baseline.plan_db_path, actual.plan_db_path
        ));
    }
    msgs
}

pub(super) fn compare_timings(baseline: &PhaseTimings, actual: &PhaseTimings) -> Vec<String> {
    let factor = std::env::var("RMIG_E2E_TIMING_FACTOR")
        .ok()
        .and_then(|s| s.parse::<f64>().ok())
        .unwrap_or(3.0);
    let slack_ms: i64 = std::env::var("RMIG_E2E_TIMING_SLACK_MS")
        .ok()
        .and_then(|s| s.parse().ok())
        .unwrap_or(100);

    let phases: &[(&str, i64, i64)] = &[
        (
            "parallel_wall_ms",
            baseline.parallel_wall_ms,
            actual.parallel_wall_ms,
        ),
        ("diff_ms", baseline.diff_ms, actual.diff_ms),
        ("plan_wall_ms", baseline.plan_wall_ms, actual.plan_wall_ms),
    ];

    let mut msgs = Vec::new();
    for (name, baseline_ms, actual_ms) in phases {
        if *baseline_ms == 0 && *actual_ms == 0 {
            continue;
        }
        let ceiling = ((*baseline_ms as f64) * factor).ceil() as i64 + slack_ms;
        if *actual_ms > ceiling {
            msgs.push(format!(
                "{name}: actual={actual_ms}ms > baseline={baseline_ms}ms ceiling={ceiling}ms (factor={factor}, slack={slack_ms}ms)"
            ));
        }
    }
    msgs
}
