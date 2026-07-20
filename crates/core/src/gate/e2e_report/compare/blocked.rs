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
    let b_scaffolds = scaffold_identities(&baseline.scaffold_paths);
    let a_scaffolds = scaffold_identities(&actual.scaffold_paths);
    if b_scaffolds != a_scaffolds {
        msgs.push(format!(
            "scaffold_paths: baseline={b_scaffolds:?} actual={a_scaffolds:?}"
        ));
    }
    msgs.extend(compare_workflow_timings(&baseline.timings, &actual.timings));
    msgs
}

/// Scaffold names embed the current git short rev (`001_<rev>_auto_add.sql`)
/// and reports may record them relative or absolute, so compare by basename
/// with the rev segment masked.
fn scaffold_identities(paths: &[String]) -> Vec<String> {
    let mut ids: Vec<String> = paths.iter().map(|p| scaffold_identity(p)).collect();
    ids.sort();
    ids
}

fn scaffold_identity(path: &str) -> String {
    let name = path.rsplit('/').next().unwrap_or(path);
    let mut parts = name.splitn(3, '_');
    match (parts.next(), parts.next(), parts.next()) {
        (Some(ord), Some(rev), Some(rest))
            if !rev.is_empty() && rev.len() <= 40 && rev.bytes().all(|b| b.is_ascii_hexdigit()) =>
        {
            format!("{ord}_REV_{rest}")
        }
        _ => name.to_string(),
    }
}
