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
    if baseline.blockers != actual.blockers {
        msgs.push(format!(
            "blockers: baseline={:?} actual={:?}",
            baseline.blockers, actual.blockers
        ));
    }
    if baseline.scaffold_paths != actual.scaffold_paths {
        msgs.push(format!(
            "scaffold_paths: baseline={:?} actual={:?}",
            baseline.scaffold_paths, actual.scaffold_paths
        ));
    }
    msgs.extend(compare_workflow_timings(&baseline.timings, &actual.timings));
    msgs
}
