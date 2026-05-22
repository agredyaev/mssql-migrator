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

pub mod audit {
    pub const BOOTSTRAP_TABLES: &str = include_str!("../../../../sql/audit/bootstrap_tables.sql");
    pub const BOOTSTRAP_INDEX: &str = include_str!("../../../../sql/audit/bootstrap_index.sql");
    pub const HISTORY_EMPTY: &str = include_str!("../../../../sql/audit/history_empty_probe.sql");
    pub const LOAD_CHECKSUMS: &str =
        include_str!("../../../../sql/audit/load_checksums_openjson.sql");
    pub const INSERT_HISTORY: &str =
        include_str!("../../../../sql/audit/insert_history_openjson.sql");
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
    pub const CACHE_SAVE_META: &str =
        include_str!("../../../../sql/catalog/catalog_cache_save_meta.sql");
}

pub mod apply {
    pub const CREATE_SCHEMA: &str = include_str!("../../../../sql/apply/create_schema.sql");
    pub const BEGIN_TX: &str = include_str!("../../../../sql/apply/begin_transaction.sql");
    pub const COMMIT_TX: &str = include_str!("../../../../sql/apply/commit_transaction.sql");
    pub const ROLLBACK: &str = include_str!("../../../../sql/apply/rollback.sql");
}

pub mod lock {
    pub const ACQUIRE: &str = include_str!("../../../../sql/lock/acquire.sql");
    pub const RELEASE: &str = include_str!("../../../../sql/lock/release.sql");
}
