use super::super::timing_compare::compare_workflow_timings;
use super::super::types::E2EBlockedReport;

/// Compares two `E2EBlockedReport` values and returns a list of mismatch descriptions.
pub fn compare_e2e_blocked_reports(
    baseline: &E2EBlockedReport,
    actual: &E2EBlockedReport,
) -> Vec<String> {
    let mut msgs = Vec::new();
    if baseline.scenario != actual.scenario {
        msgs.push(format!(
            "scenario: baseline={} actual={}",
            baseline.scenario, actual.scenario
        ));
    }
    if baseline.exit_code != actual.exit_code {
        msgs.push(format!(
            "exit_code: baseline={} actual={}",
            baseline.exit_code, actual.exit_code
        ));
    }
    if baseline.blocked != actual.blocked {
        msgs.push(format!(
            "blocked: baseline={} actual={}",
            baseline.blocked, actual.blocked
        ));
    }
    if baseline.blocked && actual.scaffold_paths.is_empty() && !baseline.scaffold_paths.is_empty() {
        msgs.push("actual: expected scaffold_paths after blocked migrate".into());
    }
    if !baseline.scaffold_paths.is_empty() && actual.scaffold_paths.is_empty() {
        msgs.push(format!(
            "scaffold_paths: baseline={} actual=0",
            baseline.scaffold_paths.len()
        ));
    }
    msgs.extend(compare_workflow_timings(&baseline.timings, &actual.timings));
    msgs
}
