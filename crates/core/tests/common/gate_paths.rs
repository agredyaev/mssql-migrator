//! Prod-gate baseline path (shared by e2e + prod-gate integration).

use crate::common::repo_root;

pub fn prod_gate_baseline_path() -> std::path::PathBuf {
    repo_root().join("crates/core/tests/testdata/prod_gate/plan_baseline_empty_db.json")
}
