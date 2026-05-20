use serde::Serialize;

use crate::timings::PhaseTimings;

use super::compare::{compare_snapshots, CompareOptions, CompareResult};
use super::snapshot::PlanSnapshot;

#[derive(Debug, Clone, Serialize)]
pub struct GateResult {
    pub go: bool,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub messages: Vec<String>,
    pub compare: CompareResult,
    pub timings: PhaseTimings,
    #[serde(rename = "delta_key_count")]
    pub delta_key_count: usize,
}

pub struct GateInput {
    pub blocked: bool,
    pub timings: PhaseTimings,
    pub max_plan_wall_ms: i64,
    pub baseline: PlanSnapshot,
    pub current: PlanSnapshot,
    pub delta_keys: std::collections::HashSet<String>,
}

pub fn evaluate_gate(in_: GateInput) -> GateResult {
    let mut messages = Vec::new();
    let mut go = true;
    if in_.blocked {
        go = false;
        messages.push("plan blocked".into());
    }
    let cmp = compare_snapshots(
        &in_.baseline,
        &in_.current,
        &CompareOptions {
            delta_keys: in_.delta_keys.clone(),
            strict_unexpected: true,
        },
    );
    if !cmp.go {
        go = false;
    }
    messages.extend(cmp.messages.clone());
    messages.extend(cmp.unexpected.clone());
    if in_.max_plan_wall_ms > 0 && in_.timings.plan_wall_ms >= in_.max_plan_wall_ms {
        go = false;
        messages.push(format!(
            "plan wall SLO exceeded: {}ms >= {}ms",
            in_.timings.plan_wall_ms, in_.max_plan_wall_ms
        ));
    }
    GateResult {
        go,
        messages,
        compare: cmp,
        timings: in_.timings,
        delta_key_count: in_.delta_keys.len(),
    }
}

pub fn max_plan_wall_ms_from_env() -> i64 {
    std::env::var("RMIG_GATE_MAX_PLAN_WALL_MS")
        .ok()
        .and_then(|s| s.trim().parse().ok())
        .unwrap_or(0)
}
