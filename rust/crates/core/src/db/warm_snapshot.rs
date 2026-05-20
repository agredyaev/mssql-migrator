//! In-process plan DB snapshot for integration/SLO (reuse after L1 invalidate).

use std::sync::OnceLock;

use super::state::CatalogState;
use crate::db::state::ChecksumMap;

#[derive(Clone)]
struct Snapshot {
    digest: [u8; 32],
    checksums: ChecksumMap,
    catalog: CatalogState,
}

static SNAP: OnceLock<Snapshot> = OnceLock::new();

pub fn store(digest: [u8; 32], checksums: ChecksumMap, catalog: CatalogState) {
    if !reuse_enabled() {
        return;
    }
    let _ = SNAP.set(Snapshot {
        digest,
        checksums,
        catalog,
    });
}

pub fn reuse(digest: &[u8; 32]) -> Option<(ChecksumMap, CatalogState)> {
    let s = SNAP.get()?;
    if !reuse_enabled() {
        return None;
    }
    if s.digest != *digest {
        return None;
    }
    Some((s.checksums.clone(), s.catalog.clone()))
}

fn reuse_enabled() -> bool {
    matches!(
        std::env::var("RMIG_INTEGRATION_WARM_SNAPSHOT").as_deref(),
        Ok("1") | Ok("true") | Ok("yes")
    )
}
