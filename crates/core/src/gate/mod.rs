//! Schema verification gates, E2E validation reports, and baseline comparison.
//!
//! ### Purpose
//! Enforces CI/CD safety gates by comparing active dynamic execution plans against committed E2E baselines,
//! detecting un-migrated DDL changes, and ensuring compliance before production deployments.
//!
//! ### Architectural Context
//! - **Inputs**: `MigrationPlan` plans, static baseline manifests, and git delta paths.
//! - **Outputs**: Parity status reports, `GateResult` evaluation codes, and E2E diff files.
//! - **Boundaries**: Operates locally during CI gates and pre-commit hooks to block unsafe deployments.
//!
//! ### Nominal Flow
//! 1. Resolve local/CI changed file paths (`resolve_changed_paths`).
//! 2. Generate active migration execution reports (`build_e2e_report`).
//! 3. Compare current schema plan vs baseline definitions (`compare_snapshots`).
//! 4. Audit execution durations against target SLO parameters (`evaluate_gate`).
//!
//! ### Off-Nominal & Failure Containment
//! - **Parity Mismatches**: If database structure differs from E2E expectations, halts verification, formats diagnostic comparison outputs, and exits with a non-zero gate code.

mod changed_paths;
mod changed_paths_ci;
mod compare;
mod delta;
mod e2e_report;
mod evaluate;
pub mod git_diff;
mod snapshot;

pub use changed_paths::{resolve_changed_paths, ChangedPathsResult};
pub use delta::{expand_delta_closure, keys_for_changed_paths};

pub use compare::{
    compare_snapshots, parity_messages, read_snapshot_json, write_snapshot_file, CompareOptions,
    CompareResult,
};
pub use e2e_report::{
    build_e2e_report, compare_e2e_apply_reports, compare_e2e_blocked_reports,
    compare_e2e_gate_reports, compare_e2e_reports, read_e2e_apply_json, read_e2e_blocked_json,
    read_e2e_gate_json, read_e2e_report_json, write_e2e_apply_file, write_e2e_blocked_file,
    write_e2e_gate_file, write_e2e_report_file, E2EApplyReport, E2EBlockedReport, E2EGateReport,
    E2EScenarioReport, E2EWorkflowTimings,
};
pub use evaluate::{evaluate_gate, max_plan_wall_ms_from_env, GateInput, GateResult};
pub use snapshot::{PlanSnapshot, SnapshotObject, SNAPSHOT_VERSION};
