//! In-process plan DB snapshot for integration/SLO (reuse after L1 invalidate).
//!
//! ### Non-obvious
//! - Gated by `RMIG_INTEGRATION_WARM_SNAPSHOT=1` env var. Without it all
//!   functions are no-ops.
//! - Uses a process-global `Mutex<Option<Snapshot>>`. A poisoned mutex disables
//!   snapshot reuse for that call instead of crashing the process.
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
    match SNAP.lock() {
        Ok(mut snap) => {
            *snap = Some(Snapshot {
                server_database: server_database.to_string(),
                digest,
                checksums,
                catalog,
            });
        }
        Err(err) => {
            tracing::warn!(error = %err, "warm snapshot store skipped after poisoned mutex");
        }
    }
}

pub fn reuse(server_database: &str, digest: &[u8; 32]) -> Option<(ChecksumMap, CatalogState)> {
    if !reuse_enabled() {
        return None;
    }
    let s = match SNAP.lock() {
        Ok(snap) => snap.clone()?,
        Err(err) => {
            tracing::warn!(error = %err, "warm snapshot reuse skipped after poisoned mutex");
            return None;
        }
    };
    if s.server_database != server_database || s.digest != *digest {
        return None;
    }
    Some((s.checksums, s.catalog))
}

/// Drop cached plan DB state (e.g. after migrate apply when catalog changed).
pub fn clear() {
    match SNAP.lock() {
        Ok(mut snap) => {
            *snap = None;
        }
        Err(err) => {
            tracing::warn!(error = %err, "warm snapshot clear skipped after poisoned mutex");
        }
    }
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
