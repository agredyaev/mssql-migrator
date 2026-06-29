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
    /// Wire keys for L1 JSON come from [`Self::insert_normalized`] or deserialize.
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

/// Serializes as `HashMap<String, hex-digest>` using normalized keys retained
/// during insert.  Each `[u8; 32]` digest is emitted as a 64-char hex string
/// (one JSON string per entry, not a 32-element number array) — far cheaper to
/// (de)serialize and ~half the bytes.  Keys are normalised `schema/kind/name`.
impl Serialize for ChecksumMap {
    fn serialize<S: Serializer>(&self, serializer: S) -> Result<S::Ok, S::Error> {
        use serde::ser::SerializeMap;
        let mut map = serializer.serialize_map(Some(self.by_fp.len()))?;
        let mut hex_buf = [0u8; 64];
        for (fp, cs) in &self.by_fp {
            let key = self.key_by_fp.get(fp).map(|k| k.as_ref()).unwrap_or("");
            hex::encode_to_slice(cs, &mut hex_buf).map_err(serde::ser::Error::custom)?;
            let digest = std::str::from_utf8(&hex_buf).map_err(serde::ser::Error::custom)?;
            map.serialize_entry(key, digest)?;
        }
        map.end()
    }
}

/// Deserializes from `HashMap<String, hex-digest>` and normalises keys on insert
/// via [`ChecksumMap::insert_normalized`].
impl<'de> Deserialize<'de> for ChecksumMap {
    fn deserialize<D: Deserializer<'de>>(deserializer: D) -> Result<Self, D::Error> {
        let wire = HashMap::<String, String>::deserialize(deserializer)?;
        let mut out = ChecksumMap::new();
        out.reserve(wire.len());
        let mut digest = [0u8; 32];
        for (k, v) in wire {
            hex::decode_to_slice(&v, &mut digest).map_err(serde::de::Error::custom)?;
            out.insert_normalized(&k, digest);
        }
        Ok(out)
    }
}
