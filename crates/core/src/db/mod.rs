//! SQL Server catalog metadata inspection and database execution plans.
//!
//! ### Purpose
//! Inspects system tables inside SQL Server (e.g. `sys.objects`, `sys.columns`) to extract the
//! current database state, mapping it to structural catalog representations for diff planning.
//!
//! ### Architectural Context
//! - **Inputs**: SQL database connections, L1 filesystem cache.
//! - **Outputs**: `CatalogState` structs containing columns, schemas, and object definitions.
//! - **Boundaries**: Uses thread-safe memory snapshots to speed up structural audits during heavy runs.
//!
//! ### Nominal Flow
//! 1. Open SQL Server connection.
//! 2. Retrieve catalog schemas, tables, and views (`run_plan_db_phase`).
//! 3. Extract detailed column structures for active tables (`load_table_columns`).
//! 4. Save metadata snapshots on disk to avoid future inspect round-trips (`save`).
//!
//! ### Off-Nominal & Failure Containment
//! - **Inspect exceptions**: If catalog loading fails (e.g., lack of metadata permissions), falls back safely, logs exceptions, and returns `Error::Sql`.

pub mod batch;
mod catalog;
mod catalog_cache;
mod catalog_cache_save;
mod catalog_inspect_cache;
mod columns;
pub mod warm_snapshot;

pub use catalog_cache::invalidate;
pub use catalog_cache_save::{save, save_batched, save_workspace_snapshot};
pub use columns::load_table_columns;
mod plan_batch;
mod plan_common;
mod plan_db_trace;
mod plan_parallel;
mod plan_snapshot;

mod checksum_map;
pub mod state;

pub use crate::domain::key_fingerprint;
pub use checksum_map::ChecksumMap;
pub use plan_db_trace::{
    max_parallel_wall_ms, maybe_append_trace, plan_db_path_from_label, plan_db_slo_exempt,
    trace_enabled, PlanDbFlags, PlanDbPath, PlanDbTimings, PlanDbTrace,
};
pub use plan_snapshot::run_plan_db_phase;
pub use state::{
    catalog_object, catalog_object_parts, intern_catalog_state, CatalogState, TableColumn,
};

pub fn invalidate_inspect_cache(db_fp: &str) {
    catalog_inspect_cache::invalidate_db(db_fp);
}
