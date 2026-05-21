use std::collections::HashMap;

use serde::{Deserialize, Serialize};

use crate::driver::IoProfile;
use crate::export::MigrationPlan;
use crate::timings::PhaseTimings;

use super::snapshot::PlanSnapshot;

#[derive(Debug, Default, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
pub struct E2EScenarioReport {
    pub scenario: String,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub setup_steps: Vec<String>,
    pub timings: PhaseTimings,
    pub io: IoProfile,
    pub snapshot: PlanSnapshot,
    pub action_counts: HashMap<String, i32>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
pub struct E2EApplyReport {
    pub scenario: String,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub setup_steps: Vec<String>,
    pub applied: i32,
    pub failed: i32,
    pub skipped: i32,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub errors: Vec<String>,
    pub audit_object_rows: i32,
}

#[derive(Debug, Default, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
pub struct E2EGateReport {
    pub scenario: String,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub setup_steps: Vec<String>,
    pub gate_go: bool,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub messages: Vec<String>,
    pub snapshot: PlanSnapshot,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
pub struct E2EBlockedReport {
    pub scenario: String,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub setup_steps: Vec<String>,
    pub exit_code: i32,
    pub blocked: bool,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub blockers: Vec<String>,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub scaffold_paths: Vec<String>,
}

pub fn action_counts_from_plan(plan: &MigrationPlan) -> HashMap<String, i32> {
    let mut out = HashMap::new();
    if !plan.rows.is_empty() {
        for row in &plan.rows {
            let action = serde_json::to_string(&row.planned_action())
                .unwrap_or_default()
                .trim_matches('"')
                .to_string();
            *out.entry(action).or_insert(0) += 1;
        }
        return out;
    }
    for obj in &plan.objects {
        let action = serde_json::to_string(&obj.planned_action)
            .unwrap_or_default()
            .trim_matches('"')
            .to_string();
        *out.entry(action).or_insert(0) += 1;
    }
    out
}

pub fn build_e2e_report(
    scenario: &str,
    plan: &MigrationPlan,
    timings: &PhaseTimings,
    io: &IoProfile,
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
    serde_json::from_str(data)
}

pub fn read_e2e_apply_json(data: &str) -> Result<E2EApplyReport, serde_json::Error> {
    serde_json::from_str(data)
}

pub fn read_e2e_gate_json(data: &str) -> Result<E2EGateReport, serde_json::Error> {
    serde_json::from_str(data)
}

pub fn read_e2e_blocked_json(data: &str) -> Result<E2EBlockedReport, serde_json::Error> {
    serde_json::from_str(data)
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

/// Compare Go reference report vs Rust run: behavior (actions + snapshot) + phase timings.
pub fn compare_e2e_reports(go: &E2EScenarioReport, rust: &E2EScenarioReport) -> Vec<String> {
    let mut msgs = Vec::new();
    if go.scenario != rust.scenario {
        msgs.push(format!(
            "scenario name: go={} rust={}",
            go.scenario, rust.scenario
        ));
    }
    if go.action_counts != rust.action_counts {
        msgs.push(format!(
            "action_counts: go={:?} rust={:?}",
            go.action_counts, rust.action_counts
        ));
    }
    msgs.extend(super::compare::parity_messages(
        &go.snapshot,
        &rust.snapshot,
    ));
    msgs.extend(compare_timings(&go.timings, &rust.timings));
    msgs
}

pub fn compare_e2e_apply_reports(go: &E2EApplyReport, rust: &E2EApplyReport) -> Vec<String> {
    let mut msgs = Vec::new();
    if go.scenario != rust.scenario {
        msgs.push(format!(
            "scenario: go={} rust={}",
            go.scenario, rust.scenario
        ));
    }
    if go.applied != rust.applied {
        msgs.push(format!("applied: go={} rust={}", go.applied, rust.applied));
    }
    if go.failed != rust.failed {
        msgs.push(format!("failed: go={} rust={}", go.failed, rust.failed));
    }
    if go.skipped != rust.skipped {
        msgs.push(format!("skipped: go={} rust={}", go.skipped, rust.skipped));
    }
    if go.audit_object_rows != rust.audit_object_rows {
        msgs.push(format!(
            "audit_object_rows: go={} rust={}",
            go.audit_object_rows, rust.audit_object_rows
        ));
    }
    msgs
}

pub fn compare_e2e_gate_reports(go: &E2EGateReport, rust: &E2EGateReport) -> Vec<String> {
    let mut msgs = Vec::new();
    if go.scenario != rust.scenario {
        msgs.push(format!(
            "scenario: go={} rust={}",
            go.scenario, rust.scenario
        ));
    }
    if go.gate_go != rust.gate_go {
        msgs.push(format!("gate_go: go={} rust={}", go.gate_go, rust.gate_go));
    }
    msgs.extend(super::compare::parity_messages(
        &go.snapshot,
        &rust.snapshot,
    ));
    msgs
}

pub fn compare_e2e_blocked_reports(go: &E2EBlockedReport, rust: &E2EBlockedReport) -> Vec<String> {
    let mut msgs = Vec::new();
    if go.scenario != rust.scenario {
        msgs.push(format!(
            "scenario: go={} rust={}",
            go.scenario, rust.scenario
        ));
    }
    if go.exit_code != rust.exit_code {
        msgs.push(format!(
            "exit_code: go={} rust={}",
            go.exit_code, rust.exit_code
        ));
    }
    if go.blocked != rust.blocked {
        msgs.push(format!("blocked: go={} rust={}", go.blocked, rust.blocked));
    }
    if go.blocked && rust.scaffold_paths.is_empty() && !go.scaffold_paths.is_empty() {
        msgs.push("rust: expected scaffold_paths after blocked migrate".into());
    }
    if !go.scaffold_paths.is_empty() && rust.scaffold_paths.is_empty() {
        msgs.push(format!(
            "scaffold_paths: go={} rust=0",
            go.scaffold_paths.len()
        ));
    }
    msgs
}

fn compare_timings(go: &PhaseTimings, rust: &PhaseTimings) -> Vec<String> {
    let factor = std::env::var("RMIG_E2E_TIMING_FACTOR")
        .ok()
        .and_then(|s| s.parse::<f64>().ok())
        .unwrap_or(3.0);
    let slack_ms: i64 = std::env::var("RMIG_E2E_TIMING_SLACK_MS")
        .ok()
        .and_then(|s| s.parse().ok())
        .unwrap_or(100);

    let phases: &[(&str, i64, i64)] = &[
        (
            "parallel_wall_ms",
            go.parallel_wall_ms,
            rust.parallel_wall_ms,
        ),
        ("diff_ms", go.diff_ms, rust.diff_ms),
        ("plan_wall_ms", go.plan_wall_ms, rust.plan_wall_ms),
    ];

    let mut msgs = Vec::new();
    for (name, go_ms, rust_ms) in phases {
        if *go_ms == 0 && *rust_ms == 0 {
            continue;
        }
        let ceiling = ((*go_ms as f64) * factor).ceil() as i64 + slack_ms;
        if *rust_ms > ceiling {
            msgs.push(format!(
                "{name}: rust={rust_ms}ms > go={go_ms}ms ceiling={ceiling}ms (factor={factor}, slack={slack_ms}ms)"
            ));
        }
    }
    msgs
}
