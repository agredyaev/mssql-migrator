mod changed_paths;
mod changed_paths_ci;
mod compare;
mod delta;
mod e2e_report;
mod evaluate;
pub mod git_diff;
pub mod repo_root;
mod snapshot;

pub use changed_paths::{resolve_changed_paths, ChangedPathsResult};
pub use delta::{expand_delta_closure, keys_for_changed_paths};

pub use compare::{
    compare_snapshots, parity_messages, read_snapshot_json, write_snapshot_file,
    write_snapshot_json, CompareOptions, CompareResult,
};
pub use e2e_report::{
    action_counts_from_plan, build_e2e_report, compare_e2e_apply_reports,
    compare_e2e_blocked_reports, compare_e2e_gate_reports, compare_e2e_reports,
    read_e2e_apply_json, read_e2e_blocked_json, read_e2e_gate_json, read_e2e_report_json,
    write_e2e_apply_file, write_e2e_blocked_file, write_e2e_gate_file, write_e2e_report_file,
    E2EApplyReport, E2EBlockedReport, E2EGateReport, E2EScenarioReport, E2EWorkflowTimings,
};
pub use evaluate::{evaluate_gate, max_plan_wall_ms_from_env, GateInput, GateResult};
pub use snapshot::{PlanSnapshot, SnapshotObject, SNAPSHOT_VERSION};
