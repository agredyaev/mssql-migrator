//! Pins on embedded SQL asset text: column widths and shapes that Rust-side
//! identity/audit code depends on.

use super::{apply, audit, catalog};

/// Audit identity columns must be NVARCHAR: VARCHAR narrows valid Unicode
/// identifiers/authors into `?` on non-UTF8 collations, collapsing distinct
/// keys into one.
#[test]
fn audit_identity_columns_are_nvarchar_regression() {
    assert!(
        audit::BOOTSTRAP_TABLES.contains("normalized_key  NVARCHAR(512)"),
        "history.normalized_key must be NVARCHAR(512)"
    );
    assert!(
        audit::BOOTSTRAP_TABLES.contains("git_author      NVARCHAR(256)"),
        "history.git_author must be NVARCHAR(256)"
    );
    assert!(
        audit::BOOTSTRAP_TABLES
            .contains("normalized_key  NVARCHAR(512) NOT NULL CONSTRAINT PK_catalog_cache"),
        "catalog_cache.normalized_key must be NVARCHAR(512)"
    );
    assert!(
        audit::INSERT_HISTORY.contains("normalized_key nvarchar(512)"),
        "OPENJSON projection must not narrow the key"
    );
    assert!(
        catalog::CACHE_INSERT.contains("normalized_key NVARCHAR(512)"),
        "cache OPENJSON projection must not narrow the key"
    );
}

/// SHA-256 Git repositories emit 64-hex commit ids; a 40-char column silently
/// truncates them.
#[test]
fn git_hash_columns_fit_sha256_commits_regression() {
    assert!(
        audit::BOOTSTRAP_TABLES.contains("git_hash        VARCHAR(64)"),
        "history.git_hash must fit 64 hex chars"
    );
    assert!(
        audit::INSERT_HISTORY.contains("git_hash       nvarchar(64)"),
        "OPENJSON git_hash projection must fit 64 hex chars"
    );
}

/// Applied-transition loading must return the recorded checksum so edited
/// applied scripts are detected instead of silently filtered.
#[test]
fn load_all_migrations_selects_checksum_regression() {
    assert!(
        audit::LOAD_ALL_MIGRATIONS.contains("h.checksum"),
        "must select the checksum column"
    );
    assert!(
        audit::LOAD_ALL_MIGRATIONS.contains("MAX(h2.id)"),
        "must take the latest row per key"
    );
}

/// The executor asserts its transaction is still open after the script body,
/// so a body-issued COMMIT/ROLLBACK cannot detach the history write.
#[test]
fn assert_open_tx_guards_trancount_regression() {
    assert!(apply::ASSERT_OPEN_TX.contains("@@TRANCOUNT"));
    assert!(apply::ASSERT_OPEN_TX.contains("THROW"));
}

/// Concurrent unlocked bootstraps race check-then-create; duplicate-create
/// errors must be tolerated instead of failing the first deployment.
#[test]
fn bootstrap_tolerates_concurrent_duplicate_creates_regression() {
    assert!(
        audit::BOOTSTRAP_TABLES.contains("BEGIN TRY"),
        "bootstrap creates must be guarded"
    );
    assert!(
        audit::BOOTSTRAP_TABLES.contains("ERROR_NUMBER() NOT IN (2714, 2759)"),
        "schema duplicate errors are tolerated"
    );
    assert!(
        audit::BOOTSTRAP_TABLES.contains("ERROR_NUMBER() <> 2714"),
        "table duplicate errors are tolerated"
    );
}

/// Cache rows must bind to the same layout digest as the metadata row, or a
/// torn concurrent save serves rows from one layout under another's meta.
#[test]
fn catalog_cache_load_binds_rows_to_digest_regression() {
    assert!(catalog::CACHE_LOAD.contains("c.layout_digest = m.layout_digest"));
}

/// Alias user-defined types live in sys.types (not sys.table_types); the
/// catalog query must see both forms or alias types are re-created forever.
#[test]
fn catalog_types_covers_alias_types_regression() {
    assert!(catalog::TYPES.contains("sys.table_types"));
    assert!(catalog::TYPES.contains("sys.types"));
    assert!(catalog::TYPES.contains("is_table_type = 0"));
}

/// Index rows must carry the parent table so same-named indexes on different
/// tables are detectable as ambiguous.
#[test]
fn catalog_indexes_return_parent_regression() {
    assert!(catalog::INDEXES.contains("LOWER(o.name) AS parent_name"));
}
