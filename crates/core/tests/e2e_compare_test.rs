//! E2E baseline comparators must cover every contract field: an ignored field
//! is a regression class that merges behind a green gate.

use migrator_core::gate::{
    compare_e2e_apply_reports, compare_e2e_blocked_reports, compare_e2e_gate_reports,
    compare_e2e_reports, E2EApplyReport, E2EBlockedReport, E2EGateReport, E2EScenarioReport,
};

#[test]
fn scenario_compare_flags_io_and_setup_changes_regression() {
    let baseline = E2EScenarioReport::default();
    let mut with_io = E2EScenarioReport::default();
    with_io.io.query_calls = 999;
    let msgs = compare_e2e_reports(&baseline, &with_io);
    assert!(
        msgs.iter().any(|m| m.contains("io.query_calls")),
        "query-call regressions must be visible: {msgs:?}"
    );

    let with_setup = E2EScenarioReport {
        setup_steps: vec!["extra".into()],
        ..Default::default()
    };
    let msgs = compare_e2e_reports(&baseline, &with_setup);
    assert!(
        msgs.iter().any(|m| m.contains("setup_steps")),
        "setup drift must be visible: {msgs:?}"
    );
}

#[test]
fn apply_compare_flags_errors_regression() {
    let baseline = E2EApplyReport::default();
    let actual = E2EApplyReport {
        errors: vec!["boom".into()],
        ..Default::default()
    };
    let msgs = compare_e2e_apply_reports(&baseline, &actual);
    assert!(
        msgs.iter().any(|m| m.contains("errors")),
        "apply errors must be compared: {msgs:?}"
    );
}

#[test]
fn blocked_compare_flags_blockers_and_scaffold_content_regression() {
    let baseline = E2EBlockedReport {
        blockers: vec!["r/tables/t1".into()],
        scaffold_paths: vec!["a.sql".into()],
        ..Default::default()
    };
    let actual = E2EBlockedReport {
        blockers: vec!["r/tables/OTHER".into()],
        scaffold_paths: vec!["b.sql".into()],
        ..Default::default()
    };
    let msgs = compare_e2e_blocked_reports(&baseline, &actual);
    assert!(
        msgs.iter().any(|m| m.contains("blockers")),
        "blocker drift must be visible: {msgs:?}"
    );
    assert!(
        msgs.iter().any(|m| m.contains("scaffold_paths")),
        "scaffold CONTENT (not just count) must be compared: {msgs:?}"
    );
}

#[test]
fn gate_compare_flags_message_changes_regression() {
    let baseline = E2EGateReport::default();
    let actual = E2EGateReport {
        messages: vec!["risky action".into()],
        ..Default::default()
    };
    let msgs = compare_e2e_gate_reports(&baseline, &actual);
    assert!(
        msgs.iter().any(|m| m.contains("messages")),
        "gate diagnostics must be compared: {msgs:?}"
    );
}

/// A zero measurement against a nonzero baseline is MISSING data, not an
/// improvement.
#[test]
fn apply_compare_flags_missing_timing_regression() {
    let mut baseline = E2EApplyReport::default();
    baseline.timings.total_ms = 1000;
    baseline.timings.plan_wall_ms = 500;
    let mut actual = E2EApplyReport::default();
    actual.timings.total_ms = 900;
    actual.timings.plan_wall_ms = 0;
    let msgs = compare_e2e_apply_reports(&baseline, &actual);
    assert!(
        msgs.iter().any(|m| m.contains("missing measurement")),
        "zero-vs-nonzero timing must fail: {msgs:?}"
    );
}
