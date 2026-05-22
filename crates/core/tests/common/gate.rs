//! Prod-gate helpers (used by `prod_gate_integration`).

pub use super::gate_paths::prod_gate_baseline_path;

pub fn gate_update_baseline() -> bool {
    matches!(
        std::env::var("RMIG_GATE_UPDATE_BASELINE").as_deref(),
        Ok("1") | Ok("true") | Ok("yes")
    )
}
