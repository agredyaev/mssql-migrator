//! Serialized JSON migration execution reports, plan files, and schema comparisons.
//!
//! ### Purpose
//! Serializes resolved layout execution plans and execution run outcomes to target file locations in
//! standard JSON format to support human inspection and automated CI/CD validation.
//!
//! ### Architectural Context
//! - **Inputs**: `MigrationPlan` layouts, transaction execution logs.
//! - **Outputs**: Output report structures (`.plan.json` and `.report.json`).
//! - **Boundaries**: Operates statelessly by writing to standard system files.
//!
//! ### Nominal Flow
//! 1. Map planned migrations and transitions to target JSON format.
//! 2. Write structural output plans to disk (`write_plan_json`).
//! 3. Write detailed run results on completion (`write_reports`).
//!
//! ### Off-Nominal & Failure Containment
//! - **I/O Exceptions**: If file writing fails, captures the error, logs it as a warning, and avoids interrupting standard database catalog changes.

mod checksum_json;
mod materialize;
mod plan_json;
mod report;

pub use materialize::{filter_applied_migrations_on_plan, PlanJsonFromObjects};
pub use plan_json::{
    write_plan_json, MigrationPlan, PlanSummary, PlannedGit, PlannedObject, PlannedSchema,
};
pub use report::{write_reports, RunFinished};
