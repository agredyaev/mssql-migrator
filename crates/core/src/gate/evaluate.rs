//! Gate evaluation: compares plan snapshots and enforces SLOs for go/no-go.
//!
//! ### Purpose
//! `evaluate_gate` takes a baseline snapshot, the current plan snapshot, and
//! delta keys, then produces a `GateResult` with pass/fail status, comparison
//! messages, and wall-time SLO enforcement.
//!
//! ### Nominal flow
//! 1. Check `blocked` flag → fail gate if true.
//! 2. `compare_snapshots(baseline, current, options)` → detect unexpected diffs.
//! 3. Enforce `max_plan_wall_ms` SLO.
//! 4. Return `GateResult { passed, messages, compare, … }`.
//!
//! ### Environment
//! - `RMIG_GATE_MAX_PLAN_WALL_MS` — optional SLO threshold (ms).

use serde::Serialize;

use crate::timings::PhaseTimings;

use super::compare::{compare_snapshots, CompareOptions, CompareResult};
use super::snapshot::PlanSnapshot;

/// Result of a gate evaluation: pass/fail with diagnostic messages.
#[derive(Debug, Clone, Serialize)]
pub struct GateResult {
    /// True when the gate check passed (no blockers, no snapshot diffs, SLO met).
    pub passed: bool,
    /// Diagnostic messages explaining failures.
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub messages: Vec<String>,
    /// Detailed snapshot comparison result.
    pub compare: CompareResult,
    /// Phase timings from the plan run.
    pub timings: PhaseTimings,
    /// Number of delta keys provided for the comparison.
    #[serde(rename = "delta_key_count")]
    pub delta_key_count: usize,
}

/// Input parameters for a single gate evaluation.
pub struct GateInput {
    /// True when the plan was blocked.
    pub blocked: bool,
    /// Phase timings from the plan run.
    pub timings: PhaseTimings,
    /// Maximum allowed plan wall time in ms (0 = no limit).
    pub max_plan_wall_ms: i64,
    /// Previously committed baseline snapshot.
    pub baseline: PlanSnapshot,
    /// Current plan snapshot to compare against baseline.
    pub current: PlanSnapshot,
    /// Keys expected to differ between baseline and current.
    pub delta_keys: std::collections::HashSet<String>,
}

/// Evaluate the incremental gate: compare snapshots, enforce SLOs → `GateResult`.
pub fn evaluate_gate(in_: GateInput) -> GateResult {
    let mut messages = Vec::new();
    let mut passed = true;
    if in_.blocked {
        passed = false;
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
    if !cmp.passed {
        passed = false;
    }
    messages.extend(cmp.messages.clone());
    messages.extend(cmp.unexpected.clone());
    if in_.max_plan_wall_ms > 0 && in_.timings.plan_wall_ms >= in_.max_plan_wall_ms {
        passed = false;
        messages.push(format!(
            "plan wall SLO exceeded: {}ms >= {}ms",
            in_.timings.plan_wall_ms, in_.max_plan_wall_ms
        ));
    }
    GateResult {
        passed,
        messages,
        compare: cmp,
        timings: in_.timings,
        delta_key_count: in_.delta_keys.len(),
    }
}

/// Read `RMIG_GATE_MAX_PLAN_WALL_MS` from the environment.
///
/// Unset/empty → `Ok(0)` (SLO disabled). A PRESENT value must be a positive
/// integer: a typo like `4s`, `-1`, or `0` would otherwise silently disable a
/// production release safeguard, so it fails closed instead.
pub fn max_plan_wall_ms_from_env() -> std::result::Result<i64, String> {
    let raw = match std::env::var("RMIG_GATE_MAX_PLAN_WALL_MS") {
        Ok(v) if !v.trim().is_empty() => v,
        _ => return Ok(0),
    };
    match raw.trim().parse::<i64>() {
        Ok(n) if n > 0 => Ok(n),
        _ => Err(format!(
            "RMIG_GATE_MAX_PLAN_WALL_MS must be a positive integer (ms), got {raw:?}"
        )),
    }
}
