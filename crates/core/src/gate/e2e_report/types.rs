//! Wire types for E2E scenario reports (plan, apply, gate, blocked, timings).
//!
//! ### Purpose
//! Defines the serialised report structures produced by the E2E gate harness
//! when running full-plan → apply → gate scenarios against an MSSQL instance.
//! Each report variant captures scenario name, setup steps, timings, and
//! scenario-specific result data.

use std::collections::HashMap;

use serde::{Deserialize, Serialize};

use crate::driver::IoProfile;
use crate::timings::PhaseTimings;

use crate::gate::snapshot::PlanSnapshot;

/// Full E2E plan-scenario report: timings, I/O profile, snapshot, action counts.
#[derive(Debug, Default, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
pub struct E2EScenarioReport {
    /// Scenario label (e.g. `empty_db_plan`, `warm_db_plan`).
    pub scenario: String,
    /// Shell commands run before the scenario for setup.
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub setup_steps: Vec<String>,
    /// Phase-level wall timings for this scenario run.
    pub timings: PhaseTimings,
    /// I/O profile (TDS round-trips, bytes).
    pub io: IoProfile,
    /// Baseline snapshot produced by the plan.
    pub snapshot: PlanSnapshot,
    /// Map of action type → count (create, adopt, skip, …).
    pub action_counts: HashMap<String, i32>,
}

/// E2E apply-scenario report: row counts, errors, workflow timings.
#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
pub struct E2EApplyReport {
    /// Scenario label.
    pub scenario: String,
    /// Setup steps executed before the apply.
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub setup_steps: Vec<String>,
    /// Number of objects successfully applied.
    pub applied: i32,
    /// Number of objects that failed to apply.
    pub failed: i32,
    /// Number of objects skipped during apply.
    pub skipped: i32,
    /// Error messages from failed applies.
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub errors: Vec<String>,
    /// Rows in the audit object table after apply.
    pub audit_object_rows: i32,
    /// Rows in the audit migration table after apply.
    #[serde(default)]
    pub audit_migration_rows: i32,
    /// Rows in the catalog metadata table after apply.
    #[serde(default)]
    pub catalog_meta_rows: i32,
    /// Rows in the catalog cache after apply.
    #[serde(default)]
    pub catalog_cache_rows: i32,
    /// End-to-end workflow timings.
    #[serde(default)]
    pub timings: E2EWorkflowTimings,
}

/// E2E gate-scenario report: pass/fail status, messages, and the snapshot used.
#[derive(Debug, Default, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
pub struct E2EGateReport {
    /// Scenario label.
    pub scenario: String,
    /// Setup steps run before the gate check.
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub setup_steps: Vec<String>,
    /// Whether the gate check passed.
    pub gate_pass: bool,
    /// Diagnostic messages from gate evaluation.
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub messages: Vec<String>,
    /// Snapshot that was compared against baseline.
    pub snapshot: PlanSnapshot,
}

/// Wall-clock timings for an end-to-end workflow (setup + plan + migrate).
#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
pub struct E2EWorkflowTimings {
    /// Cold apply / baseline setup before git delta plan.
    pub setup_apply_ms: i64,
    /// Wall time for the plan phase.
    pub plan_wall_ms: i64,
    /// Parallel wall time for plan-DB queries.
    pub plan_parallel_wall_ms: i64,
    /// Path to the plan-DB trace file.
    pub plan_db_path: String,
    /// Wall time for the migrate phase.
    pub migrate_wall_ms: i64,
    /// Parallel wall time for migrate-DB queries.
    pub migrate_parallel_wall_ms: i64,
    /// Total wall time across all phases.
    pub total_ms: i64,
}

/// E2E blocked-scenario report: blocker descriptions and scaffold paths.
#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
pub struct E2EBlockedReport {
    /// Scenario label.
    pub scenario: String,
    /// Setup steps before the scenario.
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub setup_steps: Vec<String>,
    /// Process exit code from the scenario run.
    pub exit_code: i32,
    /// True when the plan was blocked.
    pub blocked: bool,
    /// Human-readable blocker descriptions.
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub blockers: Vec<String>,
    /// Relative scaffold paths generated for blocked transitions.
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub scaffold_paths: Vec<String>,
    /// Workflow timings for the blocked scenario.
    pub timings: E2EWorkflowTimings,
}
