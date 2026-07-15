use std::fs;
use std::path::PathBuf;

use serde::{Deserialize, Serialize};

use crate::db::state::{CatalogState, ChecksumMap};
use crate::error::{Error, Result};
use crate::session::limits::MAX_L1_CACHE_BYTES;

/// Loaded L1 cache entry: checksum map paired with catalog state.
pub type L1Hit = (ChecksumMap, CatalogState);

#[derive(Serialize, Deserialize, Default)]
struct L1Payload {
    layout_digest: [u8; 32],
    checksums: ChecksumMap,
    catalog: CatalogState,
}

/// On-disk L1 cache that stores pre-computed checksums and catalog state keyed by layout digest.
pub struct L1Cache {
    root: PathBuf,
}

impl L1Cache {
    /// Creates a new `L1Cache` rooted at `root`.
    pub fn new(root: &str) -> Self {
        Self {
            root: PathBuf::from(root),
        }
    }

    /// Loads a cached entry for `fingerprint` / `digest`, returning `None` on miss, corruption, or size overflow.
    pub fn try_load(&self, fingerprint: &str, digest: &[u8; 32]) -> Result<Option<L1Hit>> {
        let path = self.path(fingerprint, digest);
        // Check size via metadata BEFORE reading, so an oversized/corrupt file is
        // never fully buffered into memory.
        let Ok(meta) = fs::metadata(&path) else {
            return Ok(None);
        };
        if !meta.is_file() || meta.len() > MAX_L1_CACHE_BYTES as u64 {
            return Ok(None);
        }
        let data = fs::read(&path).map_err(Error::Io)?;
        // Stale / corrupt / legacy-format payload: treat as a miss and rebuild
        // rather than hard-erroring — the L1 cache is regenerable and digest-keyed.
        let Ok(p) = serde_json::from_slice::<L1Payload>(&data) else {
            return Ok(None);
        };
        if p.layout_digest != *digest {
            return Ok(None);
        }
        Ok(Some((p.checksums, p.catalog)))
    }

    /// Persists `checksums` and `catalog` under `fingerprint` / `digest`.
    pub fn save(
        &self,
        fingerprint: &str,
        digest: &[u8; 32],
        checksums: &ChecksumMap,
        catalog: &CatalogState,
    ) -> Result<()> {
        let path = self.path(fingerprint, digest);
        if let Some(parent) = path.parent() {
            fs::create_dir_all(parent).map_err(Error::Io)?;
        }
        let p = L1Payload {
            layout_digest: *digest,
            checksums: checksums.clone(),
            catalog: catalog.clone(),
        };
        let data = serde_json::to_vec(&p).map_err(|e| Error::Other(e.into()))?;
        // A payload larger than the load cap would be written but never loadable
        // (a permanent silent miss); skip persisting it instead.
        if data.len() > MAX_L1_CACHE_BYTES {
            return Ok(());
        }
        // Write to a per-process temp then rename, so a concurrent reader or a
        // crash mid-write never sees a torn file.
        let tmp = path.with_extension(format!("{}.tmp", std::process::id()));
        fs::write(&tmp, &data).map_err(Error::Io)?;
        fs::rename(&tmp, &path).map_err(Error::Io)?;
        Ok(())
    }

    /// Removes all cached entries for `fingerprint`.
    pub fn invalidate_all(&self, fingerprint: &str) -> Result<()> {
        let dir = self.root.join(fingerprint);
        if dir.is_dir() {
            fs::remove_dir_all(dir).map_err(Error::Io)?;
        }
        Ok(())
    }

    fn path(&self, fingerprint: &str, digest: &[u8; 32]) -> PathBuf {
        let hex = hex::encode(digest);
        self.root.join(fingerprint).join(format!("{hex}.json"))
    }
}

#[cfg(test)]
#[path = "../tests/cache_l1_test.rs"]
mod cache_l1_tests;
