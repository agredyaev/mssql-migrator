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
