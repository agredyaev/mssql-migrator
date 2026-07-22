use crate::timings::PhaseTimings;

pub(super) fn compare_timings(baseline: &PhaseTimings, actual: &PhaseTimings) -> Vec<String> {
    let (factor, slack_ms) = super::timing_env();

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
