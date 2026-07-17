//! Audit persistence round-trips: Unicode identity keys/authors and 64-char
//! (SHA-256) Git commit ids must be stored losslessly in a fresh bootstrap.
//!
//! Run:
//!   RMIG_RUN_SQLSERVER_INTEGRATION=1 cargo test -p migrator-core --test audit_unicode_integration -- --nocapture --test-threads=1

#[path = "common/integration_enabled.rs"]
mod integration_enabled;

#[path = "common/workflow_config.rs"]
mod workflow_config;

#[path = "common/db_reset.rs"]
mod db_reset;

#[path = "common/state_smoke_conn.rs"]
mod state_smoke_conn;

use migrator_core::audit::{self, flush_history};

const CJK_KEY_A: &str = "dbo/tables/表";
const CJK_KEY_B: &str = "dbo/tables/行";
const CJK_AUTHOR: &str = "张三";
const SHA256_HASH: &str = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef";

/// Two distinct CJK keys must remain two distinct stored keys, the author must
/// round-trip exactly, and a 64-char commit id must not be truncated.
#[tokio::test(flavor = "current_thread")]
async fn unicode_keys_and_sha256_hashes_round_trip_regression() {
    if !integration_enabled::enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }
    let mut cfg = workflow_config::workflow_config().clone();
    cfg.set_skip_git(true);
    db_reset::reset_test_database(&cfg).await.expect("reset db");
    let mut conn = state_smoke_conn::open_conn(&cfg).await.expect("connect");

    let recs = [CJK_KEY_A, CJK_KEY_B].map(|key| {
        audit::record_applied(
            key,
            "tables",
            [3; 32],
            SHA256_HASH,
            CJK_AUTHOR,
            "2026-01-02T03:04:05+00:00",
            "object",
        )
    });
    flush_history(&mut conn, &recs).await.expect("insert rows");

    let rows = conn
        .query(
            "SELECT normalized_key, git_author, git_hash, LEN(git_hash) \
             FROM azdo_deploy_meta.history ORDER BY normalized_key",
            &[],
        )
        .await
        .expect("read back");
    assert_eq!(rows.len(), 2, "two distinct keys must stay two rows");
    let mut keys: Vec<&str> = rows.iter().filter_map(|r| r.get_str(0)).collect();
    keys.sort_unstable();
    let mut expected = vec![CJK_KEY_A, CJK_KEY_B];
    expected.sort_unstable();
    assert_eq!(keys, expected, "keys round-trip exactly");
    for row in &rows {
        assert_eq!(row.get_str(1), Some(CJK_AUTHOR), "author round-trips");
        assert_eq!(row.get_str(2), Some(SHA256_HASH), "sha256 id round-trips");
        assert_eq!(row.get_i32(3), Some(64), "no truncation to 40 chars");
    }
}
