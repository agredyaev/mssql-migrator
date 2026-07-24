mod compare;
mod io;
mod plan_timing_compare;
mod timing_compare;
mod types;

pub use compare::{
    compare_e2e_apply_reports, compare_e2e_blocked_reports, compare_e2e_gate_reports,
    compare_e2e_reports,
};
pub use io::{
    build_e2e_report, read_e2e_apply_json, read_e2e_blocked_json, read_e2e_gate_json,
    read_e2e_report_json, write_e2e_apply_file, write_e2e_blocked_file, write_e2e_gate_file,
    write_e2e_report_file,
};
pub use types::{
    E2EApplyReport, E2EBlockedReport, E2EGateReport, E2EScenarioReport, E2EWorkflowTimings,
};

/// Shared e2e timing tolerance knobs: multiplicative `factor`
/// (`RMIG_E2E_TIMING_FACTOR`, default 3.0) and additive `slack_ms`
/// (`RMIG_E2E_TIMING_SLACK_MS`, default 100).
fn timing_env() -> (f64, i64) {
    let factor = std::env::var("RMIG_E2E_TIMING_FACTOR")
        .ok()
        .and_then(|s| s.parse::<f64>().ok())
        .unwrap_or(3.0);
    let slack_ms: i64 = std::env::var("RMIG_E2E_TIMING_SLACK_MS")
        .ok()
        .and_then(|s| s.parse().ok())
        .unwrap_or(100);
    (factor, slack_ms)
}

/// Appends a message when `actual_ms` exceeds the tolerance ceiling derived
/// from `baseline_ms` and [`timing_env`].
fn push_ceiling(msgs: &mut Vec<String>, name: &str, baseline_ms: i64, actual_ms: i64) {
    let (factor, slack_ms) = timing_env();
    let ceiling = ((baseline_ms as f64) * factor).ceil() as i64 + slack_ms;
    if actual_ms > ceiling {
        msgs.push(format!(
            "{name}: actual={actual_ms}ms > baseline={baseline_ms}ms ceiling={ceiling}ms (factor={factor}, slack={slack_ms}ms)"
        ));
    }
}
