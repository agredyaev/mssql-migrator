use std::collections::HashMap;

use serde::{Deserialize, Serialize};

use crate::driver::IoProfile;
use crate::timings::PhaseTimings;

use crate::gate::snapshot::PlanSnapshot;

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
    #[serde(default)]
    pub audit_migration_rows: i32,
    #[serde(default)]
    pub catalog_meta_rows: i32,
    #[serde(default)]
    pub catalog_cache_rows: i32,
    #[serde(default)]
    pub timings: E2EWorkflowTimings,
}

#[derive(Debug, Default, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
pub struct E2EGateReport {
    pub scenario: String,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub setup_steps: Vec<String>,
    pub gate_pass: bool,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub messages: Vec<String>,
    pub snapshot: PlanSnapshot,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
pub struct E2EWorkflowTimings {
    /// Cold apply / baseline setup before git delta plan.
    pub setup_apply_ms: i64,
    pub plan_wall_ms: i64,
    pub plan_parallel_wall_ms: i64,
    pub plan_db_path: String,
    pub migrate_wall_ms: i64,
    pub migrate_parallel_wall_ms: i64,
    pub total_ms: i64,
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
    pub timings: E2EWorkflowTimings,
}
