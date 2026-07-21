//! Database execution logging, schema bootstrapping, and migration checksum auditing.
//!
//! ### Purpose
//! Maintains and queries the execution history and checksum index records directly in the SQL Server
//! catalog target to track migration state, detect tampering, and determine incremental work plans.
//!
//! ### Architectural Context
//! - **Inputs**: SQL execution outputs, migration definitions, database servers/databases.
//! - **Outputs**: TAMPER checks, dynamic checksum maps, initialized history structures.
//! - **Boundaries**: Operates in the database catalog under the `_rmig_` metadata tables.
//!
//! ### Nominal Flow
//! 1. Verify existence of operational schema tracking tables (`ensure_tables`).
//! 2. Load committed dynamic execution records and historical checksums (plan-side `load_checksums_plan`).
//! 3. Flush execution records upon successful migrations (`flush_history`).
//! 4. Invalidate down-level memory snapshots (`invalidate_audit_cache`).
//!
//! ### Off-Nominal & Failure Containment
//! - **History index missing**: Spawns the required schema index dynamically before retrying operations.
//! - **Checksum validation exceptions**: Detects manual alterations or drift and marks executing runs as blocked.

mod history;
mod load;
mod migrations;

pub use history::{
    ensure_history_index, flush_history, record_adopted, record_applied, HistoryRecord,
};
pub use load::{
    cache_history_empty, checksum_map_from_rows, checksum_map_from_rows_ws, db_fingerprint,
    empty_checksums_from_keys_json, ensure_tables, ensure_tables_on, history_empty_cached,
    history_known_empty, history_known_nonempty, invalidate_audit_cache,
    invalidate_audit_cache_all, looks_like_checksum_rows, mark_history_nonempty,
    mark_tables_ensured, sync_tables_ensured, tables_ensured,
};
pub use migrations::load_all_applied;
