//! Prod-gate path helpers (used by `prod_gate_integration` only).

use crate::common::repo_root;

pub fn prod_gate_baseline_path() -> std::path::PathBuf {
    repo_root().join("internal/app/testdata/prod_gate/plan_baseline_empty_db.json")
}

pub fn gate_update_baseline() -> bool {
    matches!(
        std::env::var("RMIG_GATE_UPDATE_BASELINE").as_deref(),
        Ok("1") | Ok("true") | Ok("yes")
    )
}
