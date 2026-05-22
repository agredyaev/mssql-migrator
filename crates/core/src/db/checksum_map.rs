use std::collections::HashMap;

use serde::{Deserialize, Deserializer, Serialize, Serializer};

use crate::domain::{key_fingerprint, ObjectKey, StrOff, Workspace};

/// Prior checksum digests keyed by [`key_fingerprint`] (no `ObjectKey` / `SharedStr::as_str` in hot lookups).
#[derive(Clone, Debug, Default)]
pub struct ChecksumMap {
    by_fp: HashMap<u64, [u8; 32]>,
    /// Normalized keys retained for L1 JSON round-trip (wire uses `String` keys).
    key_by_fp: HashMap<u64, Box<str>>,
}

impl ChecksumMap {
    pub fn new() -> Self {
        Self {
            by_fp: HashMap::new(),
            key_by_fp: HashMap::new(),
        }
    }

    pub fn is_empty(&self) -> bool {
        self.by_fp.is_empty()
    }

    pub fn len(&self) -> usize {
        self.by_fp.len()
    }

    pub fn reserve(&mut self, additional: usize) {
        self.by_fp.reserve(additional);
    }

    pub fn insert_normalized(&mut self, key: &str, cs: [u8; 32]) {
        let fp = key_fingerprint(key.as_bytes());
        self.by_fp.insert(fp, cs);
        self.key_by_fp.insert(fp, key.into());
    }

    /// Hot-path insert: digest only (no duplicate normalized key bytes).
    /// Wire keys for L1 JSON come from [`insert_normalized`] or deserialize.
    pub fn insert_key(&mut self, key: &ObjectKey, cs: [u8; 32]) {
        let fp = key.fingerprint();
        self.by_fp.insert(fp, cs);
        self.key_by_fp
            .entry(fp)
            .or_insert_with(|| key.as_str().into());
    }

    pub fn get_normalized(&self, key: &str) -> Option<&[u8; 32]> {
        self.by_fp.get(&key_fingerprint(key.as_bytes()))
    }

    pub fn get_key(&self, key: &ObjectKey) -> Option<&[u8; 32]> {
        self.by_fp.get(&key.fingerprint())
    }

    pub fn get_fingerprint(&self, fp: u64) -> Option<&[u8; 32]> {
        self.by_fp.get(&fp)
    }

    /// Lookup by dense row `key_off` after scan finalize.
    pub fn get_key_off(&self, ws: &Workspace, key_off: StrOff) -> Option<[u8; 32]> {
        self.get_fingerprint(ws.key_off_fingerprint(key_off))
            .copied()
    }

    /// Iterate stored digests (for [`crate::plan::scope::apply_checksums_if_needed`] via `fp_index`).
    pub fn iter_digests(&self) -> impl Iterator<Item = (u64, [u8; 32])> + '_ {
        self.by_fp.iter().map(|(&fp, &cs)| (fp, cs))
    }
}

impl Serialize for ChecksumMap {
    fn serialize<S: Serializer>(&self, serializer: S) -> Result<S::Ok, S::Error> {
        let mut wire = HashMap::with_capacity(self.by_fp.len());
        for (fp, cs) in &self.by_fp {
            let key = self.key_by_fp.get(fp).map(|k| k.as_ref()).unwrap_or("");
            wire.insert(key, *cs);
        }
        wire.serialize(serializer)
    }
}

impl<'de> Deserialize<'de> for ChecksumMap {
    fn deserialize<D: Deserializer<'de>>(deserializer: D) -> Result<Self, D::Error> {
        let wire = HashMap::<String, [u8; 32]>::deserialize(deserializer)?;
        let mut out = ChecksumMap::new();
        for (k, v) in wire {
            out.insert_normalized(&k, v);
        }
        Ok(out)
    }
}
