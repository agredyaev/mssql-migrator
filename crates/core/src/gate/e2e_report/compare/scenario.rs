use super::super::timing_compare::compare_timings;
use super::super::types::E2EScenarioReport;

/// Compare committed baseline report vs current run: behavior (actions + snapshot) + phase timings.
pub fn compare_e2e_reports(
    baseline: &E2EScenarioReport,
    actual: &E2EScenarioReport,
) -> Vec<String> {
    let mut msgs = Vec::new();
    if baseline.scenario != actual.scenario {
        msgs.push(format!(
            "scenario name: baseline={} actual={}",
            baseline.scenario, actual.scenario
        ));
    }
    if baseline.action_counts != actual.action_counts {
        msgs.push(format!(
            "action_counts: baseline={:?} actual={:?}",
            baseline.action_counts, actual.action_counts
        ));
    }
    msgs.extend(crate::gate::parity_messages(
        &baseline.snapshot,
        &actual.snapshot,
    ));
    msgs.extend(compare_timings(&baseline.timings, &actual.timings));
    msgs
}
