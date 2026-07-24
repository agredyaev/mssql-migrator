//! Schema diff planning, scenario resolution, and object filter compilation.
//!
//! ### Purpose
//! Computes structural diffs between the declared filesystem SQL schema tree and the live database
//! catalog state, generating a clean list of migrations and schema transitions to run.
//!
//! ### Architectural Context
//! - **Inputs**: `Workspace` schema definition, `CatalogState` active database structures.
//! - **Outputs**: `MigrationPlan` listing additions, modifications, and deletions.
//! - **Boundaries**: Operates statelessly in memory during the planning phase of compilation.
//!
//! ### Nominal Flow
//! 1. Resolve active git changed scopes if running incremental queries (`resolve_plan_scenario`).
//! 2. Perform a structural diff of the workspace vs the live database (`compute_diff`).
//! 3. Compile the execution list and flag unsafe DDL changes.
//!
//! ### Off-Nominal & Failure Containment
//! - **DDL Shifts Without Migration**: If a schema change is detected without a valid matching migration SQL script, marks the plan as blocked and halts migration execution (fails safe).

mod diff;
mod diff_ctx;
mod diff_decide;
mod diff_object;
mod diff_plan;
mod git_scope;
mod scenario;
mod scenario_apply;
mod scenario_resolve;
pub mod scope;
mod scope_build;
mod scope_spot_check;

pub use diff::{compute_diff, compute_diff_into};
pub use git_scope::git_hot_scope_json;
pub use scenario::{CounterKind, PlanScenario};
pub use scenario_apply::apply_scenario;
pub use scenario_resolve::{resolve_plan_scenario, ScenarioInput};
