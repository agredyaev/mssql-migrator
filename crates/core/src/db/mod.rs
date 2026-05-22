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
