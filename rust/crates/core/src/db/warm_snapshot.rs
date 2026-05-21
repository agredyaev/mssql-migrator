//! In-process plan DB snapshot for integration/SLO (reuse after L1 invalidate).

use std::sync::Mutex;

use super::state::CatalogState;
use crate::db::state::ChecksumMap;

#[derive(Clone)]
struct Snapshot {
    digest: [u8; 32],
    checksums: ChecksumMap,
    catalog: CatalogState,
}

static SNAP: Mutex<Option<Snapshot>> = Mutex::new(None);

pub fn store(digest: [u8; 32], checksums: ChecksumMap, catalog: CatalogState) {
    if !reuse_enabled() {
        return;
    }
    *SNAP.lock().expect("warm snapshot lock") = Some(Snapshot {
        digest,
        checksums,
        catalog,
    });
}

pub fn reuse(digest: &[u8; 32]) -> Option<(ChecksumMap, CatalogState)> {
    if !reuse_enabled() {
        return None;
    }
    let s = SNAP.lock().expect("warm snapshot lock").clone()?;
    if s.digest != *digest {
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
