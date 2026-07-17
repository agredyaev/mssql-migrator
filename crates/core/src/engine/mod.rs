//! Main orchestration engine, CLI command router, and run-time lifecycle controller.
//!
//! ### Purpose
//! Serves as the central coordinator of the migrator system, binding file ingestion, database
//! inspections, diff planning, scaffold validations, and transactional migration apply phases.
//!
//! ### Architectural Context
//! - **Inputs**: CLI context variables and target execution commands (`plan`, `migrate`, `validate`, `baseline`).
//! - **Outputs**: Exit codes and serialized JSON report logs (`RunOutput`).
//! - **Boundaries**: Orchestrates dependencies sequentially, shielding database operations behind strict locks.
//!
//! ### Nominal Flow
//! 1. Load context configuration.
//! 2. Open standard/session-proxy connection to SQL Server database catalog.
//! 3. Scan workspace filesystem to load sql scripts (`scan`).
//! 4. Audit current DB state vs planned layout (`plan`).
//! 5. Dispatch command execution (`run_command`), managing transactional locks if applying.
//!
//! ### Off-Nominal & Failure Containment
//! - **Process Panic / Database Aborts**: Captures errors, releases session locks safely, and formats precise diagnostic logs to stderr.

mod adopt_gate;
mod apply_run;
mod blocked;
mod filter;
mod io;
mod run;
mod warm_store;

pub use io::{print_timings_json, print_version, write_plan_stdout};
pub use run::{run_command, Command, RunOutput};
