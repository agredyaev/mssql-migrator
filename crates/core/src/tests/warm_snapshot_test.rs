use std::sync::Mutex;

use crate::db::state::{CatalogState, ChecksumMap};
use crate::db::warm_snapshot::{clear, reuse, store};

static TEST_LOCK: Mutex<()> = Mutex::new(());

struct WarmTestGuard {
    _lock: std::sync::MutexGuard<'static, ()>,
}

impl Drop for WarmTestGuard {
    fn drop(&mut self) {
        std::env::remove_var("RMIG_INTEGRATION_WARM_SNAPSHOT");
        clear();
    }
}

fn enable_warm_snapshot() -> WarmTestGuard {
    let lock = TEST_LOCK.lock().expect("warm snapshot test lock");
    std::env::set_var("RMIG_INTEGRATION_WARM_SNAPSHOT", "1");
    clear();
    WarmTestGuard { _lock: lock }
}

const DIGEST: [u8; 32] = [7u8; 32];
const OTHER_DIGEST: [u8; 32] = [9u8; 32];

#[test]
fn warm_snapshot_same_server_database_and_digest_happy_path() {
    let _guard = enable_warm_snapshot();
    store(
        "localhost_dactests",
        DIGEST,
        ChecksumMap::default(),
        CatalogState::default(),
    );
    assert!(reuse("localhost_dactests", &DIGEST).is_some());
}

#[test]
fn warm_snapshot_different_server_database_negative_path() {
    let _guard = enable_warm_snapshot();
    store(
        "localhost_dactests",
        DIGEST,
        ChecksumMap::default(),
        CatalogState::default(),
    );
    assert!(reuse("localhost_warehouse", &DIGEST).is_none());
}

#[test]
fn warm_snapshot_same_db_different_digest_edge_case() {
    let _guard = enable_warm_snapshot();
    store(
        "localhost_dactests",
        DIGEST,
        ChecksumMap::default(),
        CatalogState::default(),
    );
    assert!(reuse("localhost_dactests", &OTHER_DIGEST).is_none());
}

#[test]
fn warm_snapshot_cross_database_same_digest_regression() {
    let _guard = enable_warm_snapshot();
    let mut catalog = CatalogState::default();
    catalog.schemas.insert("dbo".into());
    store(
        "localhost_warehouse",
        DIGEST,
        ChecksumMap::default(),
        catalog,
    );
    assert!(
        reuse("localhost_dactests", &DIGEST).is_none(),
        "BG-007 regression: must not reuse foreign database catalog"
    );
}
