#![allow(missing_docs)]

use std::collections::HashMap;

use serde::{de::DeserializeOwned, Serialize};

use crate::export::MigrationPlan;
use crate::timings::PhaseTimings;

use super::types::{E2EApplyReport, E2EBlockedReport, E2EGateReport, E2EScenarioReport};
use crate::gate::snapshot::PlanSnapshot;

pub fn action_counts_from_plan(plan: &MigrationPlan) -> HashMap<String, i32> {
    let mut out = HashMap::new();
    for obj in &plan.objects {
        let action = crate::gate::action_str(&obj.planned_action);
        *out.entry(action).or_insert(0) += 1;
    }
    out
}

pub fn build_e2e_report(
    scenario: &str,
    plan: &MigrationPlan,
    timings: &PhaseTimings,
    io: &crate::driver::IoProfile,
) -> E2EScenarioReport {
    E2EScenarioReport {
        scenario: scenario.into(),
        setup_steps: Vec::new(),
        timings: timings.clone(),
        io: io.clone(),
        snapshot: PlanSnapshot::from_plan(plan),
        action_counts: action_counts_from_plan(plan),
    }
}

pub fn read_e2e_report_json(data: &str) -> Result<E2EScenarioReport, serde_json::Error> {
    read_json_object(data, &["timings", "io", "snapshot", "action_counts"])
}

pub fn read_e2e_apply_json(data: &str) -> Result<E2EApplyReport, serde_json::Error> {
    read_json_object(data, &["timings"])
}

pub fn read_e2e_gate_json(data: &str) -> Result<E2EGateReport, serde_json::Error> {
    read_json_object(data, &["snapshot"])
}

pub fn read_e2e_blocked_json(data: &str) -> Result<E2EBlockedReport, serde_json::Error> {
    read_json_object(data, &["timings"])
}

fn read_json_object<T: DeserializeOwned>(
    data: &str,
    object_fields: &[&str],
) -> Result<T, serde_json::Error> {
    let value: serde_json::Value = serde_json::from_str(data)?;
    if !value.is_object()
        || object_fields
            .iter()
            .any(|field| value.get(field).is_some_and(|nested| !nested.is_object()))
    {
        return Err(<serde_json::Error as serde::de::Error>::custom(
            "expected JSON object fields",
        ));
    }
    serde_json::from_value(value)
}

pub fn write_e2e_report_file(
    path: &std::path::Path,
    rep: &E2EScenarioReport,
) -> std::io::Result<()> {
    write_json_file(path, rep)
}

pub fn write_e2e_apply_file(path: &std::path::Path, rep: &E2EApplyReport) -> std::io::Result<()> {
    write_json_file(path, rep)
}

pub fn write_e2e_gate_file(path: &std::path::Path, rep: &E2EGateReport) -> std::io::Result<()> {
    write_json_file(path, rep)
}

pub fn write_e2e_blocked_file(
    path: &std::path::Path,
    rep: &E2EBlockedReport,
) -> std::io::Result<()> {
    write_json_file(path, rep)
}

fn write_json_file<T: Serialize>(path: &std::path::Path, rep: &T) -> std::io::Result<()> {
    let data = serde_json::to_string_pretty(rep).map_err(std::io::Error::other)?;
    if let Some(parent) = path.parent() {
        std::fs::create_dir_all(parent)?;
    }
    std::fs::write(path, format!("{data}\n"))
}
