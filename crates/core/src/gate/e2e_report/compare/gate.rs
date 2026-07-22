use super::super::types::E2EGateReport;
use super::diff_field;

/// Compares two `E2EGateReport` values and returns a list of mismatch descriptions.
pub fn compare_e2e_gate_reports(baseline: &E2EGateReport, actual: &E2EGateReport) -> Vec<String> {
    let mut msgs = Vec::new();
    diff_field(&mut msgs, "scenario", &baseline.scenario, &actual.scenario);
    diff_field(
        &mut msgs,
        "gate_pass",
        &baseline.gate_pass,
        &actual.gate_pass,
    );
    if baseline.messages != actual.messages {
        msgs.push(format!(
            "messages: baseline={:?} actual={:?}",
            baseline.messages, actual.messages
        ));
    }
    msgs.extend(crate::gate::parity_messages(
        &baseline.snapshot,
        &actual.snapshot,
    ));
    msgs
}
