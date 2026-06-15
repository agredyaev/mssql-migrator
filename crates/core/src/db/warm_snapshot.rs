//! In-process plan DB snapshot for integration/SLO (reuse after L1 invalidate).
//!
//! ### Non-obvious
//! - Gated by `RMIG_INTEGRATION_WARM_SNAPSHOT=1` env var. Without it all
//!   functions are no-ops.
//! - Uses a process-global `Mutex<Option<Snapshot>>`. Every access unwraps the
//!   lock — a panic while holding it will poison the mutex and crash subsequent
//!   callers. Acceptable because this is a test-only / SLO-gate facility.
//! - The digest + `server_database` pair are compared for cache-hit; a digest
//!   match across different databases (BG-007) is explicitly rejected.

use std::sync::Mutex;

use super::state::CatalogState;
use crate::db::state::ChecksumMap;

#[derive(Clone)]
struct Snapshot {
    server_database: String,
    digest: [u8; 32],
    checksums: ChecksumMap,
    catalog: CatalogState,
}

static SNAP: Mutex<Option<Snapshot>> = Mutex::new(None);

pub fn store(
    server_database: &str,
    digest: [u8; 32],
    checksums: ChecksumMap,
    catalog: CatalogState,
) {
    if !reuse_enabled() {
        return;
    }
    *SNAP.lock().expect("warm snapshot lock") = Some(Snapshot {
        server_database: server_database.to_string(),
        digest,
        checksums,
        catalog,
    });
}

pub fn reuse(server_database: &str, digest: &[u8; 32]) -> Option<(ChecksumMap, CatalogState)> {
    if !reuse_enabled() {
        return None;
    }
    let s = SNAP.lock().expect("warm snapshot lock").clone()?;
    if s.server_database != server_database || s.digest != *digest {
        return None;
    }
    Some((s.checksums, s.catalog))
}

/// Drop cached plan DB state (e.g. after migrate apply when catalog changed).
pub fn clear() {
    *SNAP.lock().expect("warm snapshot lock") = None;
}

fn reuse_enabled() -> bool {
    matches!(
        std::env::var("RMIG_INTEGRATION_WARM_SNAPSHOT").as_deref(),
        Ok("1") | Ok("true") | Ok("yes")
    )
}

#[cfg(test)]
#[path = "../tests/warm_snapshot_test.rs"]
mod warm_snapshot_tests;
