use super::super::plan_timing_compare::compare_timings;
use super::super::types::E2EScenarioReport;
use super::diff_field;

/// Compare committed baseline report vs current run: behavior (actions + snapshot) + phase timings.
pub fn compare_e2e_reports(
    baseline: &E2EScenarioReport,
    actual: &E2EScenarioReport,
) -> Vec<String> {
    let mut msgs = Vec::new();
    diff_field(
        &mut msgs,
        "scenario name",
        &baseline.scenario,
        &actual.scenario,
    );
    if baseline.action_counts != actual.action_counts {
        msgs.push(format!(
            "action_counts: baseline={:?} actual={:?}",
            baseline.action_counts, actual.action_counts
        ));
    }
    if baseline.setup_steps != actual.setup_steps {
        msgs.push(format!(
            "setup_steps: baseline={:?} actual={:?}",
            baseline.setup_steps, actual.setup_steps
        ));
    }
    diff_field(
        &mut msgs,
        "io.query_calls",
        &baseline.io.query_calls,
        &actual.io.query_calls,
    );
    diff_field(
        &mut msgs,
        "timings.l1_cache_hit",
        &baseline.timings.l1_cache_hit,
        &actual.timings.l1_cache_hit,
    );
    if !baseline.timings.plan_db_path.is_empty()
        && baseline.timings.plan_db_path != actual.timings.plan_db_path
    {
        msgs.push(format!(
            "timings.plan_db_path: baseline={} actual={}",
            baseline.timings.plan_db_path, actual.timings.plan_db_path
        ));
    }
    msgs.extend(crate::gate::parity_messages(
        &baseline.snapshot,
        &actual.snapshot,
    ));
    msgs.extend(compare_timings(&baseline.timings, &actual.timings));
    msgs
}
