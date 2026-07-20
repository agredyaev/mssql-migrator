#![allow(missing_docs)]
//! Embedded T-SQL query scripts compile-time bindings.
//!
//! ### Purpose
//! Binds all static `.sql` queries located under the repository root `sql/` directory to compile-time
//! constants using `include_str!`, ensuring queries are built into the binary.
//!
//! ### Architectural Context
//! - **Inputs**: SQL files from `sql/` folder at compile-time.
//! - **Outputs**: Static query string variables (`BOOTSTRAP_TABLES`, `SYS_OBJECTS`, `ACQUIRE`).
//! - **Boundaries**: Strictly maps static SQL statements. Does not execute queries directly.
//!
//! ### Nominals & Submodules
//! - **`audit`**: Bootstrapping metadata tables and reading/writing execution history records.
//! - **`catalog`**: Inspecting SQL Server catalog tables and managing local L1 DB caches.
//! - **`apply`**: Structural schema generation and explicit transactional controls.
//! - **`lock`**: Distributed app advisory lock acquisition and releases.

#[cfg(test)]
#[path = "../tests/sql_assets_test.rs"]
mod sql_assets_tests;

pub mod audit {
    pub const BOOTSTRAP_TABLES: &str = include_str!("../../../../sql/audit/bootstrap_tables.sql");
    pub const BOOTSTRAP_INDEX: &str = include_str!("../../../../sql/audit/bootstrap_index.sql");
    /// Best-effort database DDL trigger for incremental drift tracking. Run after
    /// [`BOOTSTRAP_TABLES`] (which creates the tables it writes to); its failure
    /// is tolerated and degrades the plan to full fingerprinting.
    pub const BOOTSTRAP_DRIFT: &str = include_str!("../../../../sql/audit/bootstrap_drift.sql");
    pub const HISTORY_EMPTY: &str = include_str!("../../../../sql/audit/history_empty_probe.sql");
    pub const HISTORY_EXISTS: &str = include_str!("../../../../sql/audit/history_exists.sql");
    // Header/tail fragments are not runnable alone; the shared canonical-state
    // block is embedded by both sides.
    pub const LOAD_CHECKSUMS: &str = concat!(
        include_str!("../../../../sql/audit/load_checksums_header.sql"),
        include_str!("../../../../sql/audit/_object_canonical_state.sql"),
    );
    pub const INSERT_HISTORY: &str = concat!(
        include_str!("../../../../sql/audit/insert_history_header.sql"),
        include_str!("../../../../sql/audit/_object_canonical_state.sql"),
        include_str!("../../../../sql/audit/insert_history_tail.sql"),
    );
    pub const LOAD_ALL_MIGRATIONS: &str =
        include_str!("../../../../sql/audit/load_all_migrations.sql");
}

pub mod catalog {
    pub const SCOPED_HIT: &str = include_str!("../../../../sql/catalog/catalog_scoped_hit.sql");
    pub const SCOPE_HEADER: &str = include_str!("../../../../sql/catalog/catalog_scope_header.sql");
    pub const SCHEMA_ROWS: &str = include_str!("../../../../sql/catalog/catalog_schema_rows.sql");
    pub const SYS_OBJECTS: &str = include_str!("../../../../sql/catalog/catalog_sys_objects.sql");
    pub const TYPES: &str = include_str!("../../../../sql/catalog/catalog_types.sql");
    pub const INDEXES: &str = include_str!("../../../../sql/catalog/catalog_indexes.sql");
    pub const INDEX_PARENTS: &str =
        include_str!("../../../../sql/catalog/catalog_index_parents.sql");
    pub const COLUMNS_OPENJSON: &str = include_str!("../../../../sql/catalog/columns_openjson.sql");
    pub const CACHE_LOAD: &str = include_str!("../../../../sql/catalog/catalog_cache_load.sql");
    pub const CACHE_LOAD_RELAXED: &str =
        include_str!("../../../../sql/catalog/catalog_cache_load_relaxed.sql");
    pub const CACHE_INVALIDATE: &str =
        include_str!("../../../../sql/catalog/catalog_cache_invalidate.sql");
    pub const CACHE_DELETE_ALL: &str =
        include_str!("../../../../sql/catalog/catalog_cache_delete_all.sql");
    pub const CACHE_INSERT: &str =
        include_str!("../../../../sql/catalog/catalog_cache_insert_openjson.sql");
    pub const META_MERGE: &str = include_str!("../../../../sql/catalog/catalog_meta_merge.sql");
    pub const ROWS_PROJECTION: &str = include_str!("../../../../sql/catalog/rows_projection.sql");
}

pub mod driver {
    pub const PING: &str = include_str!("../../../../sql/driver/ping.sql");
}

pub mod apply {
    pub const BEGIN_TX: &str = include_str!("../../../../sql/apply/begin_transaction.sql");
    pub const ASSERT_OPEN_TX: &str =
        include_str!("../../../../sql/apply/assert_open_transaction.sql");
    pub const COMMIT_TX: &str = include_str!("../../../../sql/apply/commit_transaction.sql");
    pub const ROLLBACK: &str = include_str!("../../../../sql/apply/rollback.sql");
    pub const ROLLBACK_IF_OPEN: &str = include_str!("../../../../sql/apply/rollback_if_open.sql");
}

pub mod lock {
    pub const ACQUIRE: &str = include_str!("../../../../sql/lock/acquire.sql");
    pub const RELEASE: &str = include_str!("../../../../sql/lock/release.sql");
    pub const RELEASE_IF_HELD: &str = include_str!("../../../../sql/lock/release_if_held.sql");
}
