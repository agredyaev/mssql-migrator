use super::super::types::E2EGateReport;

pub fn compare_e2e_gate_reports(baseline: &E2EGateReport, actual: &E2EGateReport) -> Vec<String> {
    let mut msgs = Vec::new();
    if baseline.scenario != actual.scenario {
        msgs.push(format!(
            "scenario: baseline={} actual={}",
            baseline.scenario, actual.scenario
        ));
    }
    if baseline.gate_pass != actual.gate_pass {
        msgs.push(format!(
            "gate_pass: baseline={} actual={}",
            baseline.gate_pass, actual.gate_pass
        ));
    }
    msgs.extend(crate::gate::parity_messages(
        &baseline.snapshot,
        &actual.snapshot,
    ));
    msgs
}
