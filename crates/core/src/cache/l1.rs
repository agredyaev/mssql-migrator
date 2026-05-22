use std::fs;
use std::path::PathBuf;

use serde::{Deserialize, Serialize};

use crate::db::state::{CatalogState, ChecksumMap};
use crate::error::{Error, Result};
use crate::session::limits::MAX_L1_CACHE_BYTES;

pub type L1Hit = (ChecksumMap, CatalogState);

#[derive(Serialize, Deserialize, Default)]
struct L1Payload {
    layout_digest: [u8; 32],
    checksums: ChecksumMap,
    catalog: CatalogState,
}

pub struct L1Cache {
    root: PathBuf,
}

impl L1Cache {
    pub fn new(root: &str) -> Self {
        Self {
            root: PathBuf::from(root),
        }
    }

    pub fn try_load(&self, fingerprint: &str, digest: &[u8; 32]) -> Result<Option<L1Hit>> {
        let path = self.path(fingerprint, digest);
        if !path.is_file() {
            return Ok(None);
        }
        let data = fs::read(&path).map_err(Error::Io)?;
        if data.len() > MAX_L1_CACHE_BYTES {
            return Ok(None);
        }
        let p: L1Payload = serde_json::from_slice(&data).map_err(|e| Error::Other(e.into()))?;
        if p.layout_digest != *digest {
            return Ok(None);
        }
        Ok(Some((p.checksums, p.catalog)))
    }

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
        fs::write(path, data).map_err(Error::Io)?;
        Ok(())
    }

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
