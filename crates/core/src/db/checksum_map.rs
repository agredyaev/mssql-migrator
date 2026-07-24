use std::collections::{HashMap, HashSet};

use crate::domain::ObjectKey;

/// Prior checksum digests keyed by normalized object key.
#[derive(Clone, Debug, Default)]
pub struct ChecksumMap {
    by_key: HashMap<ObjectKey, [u8; 32]>,
    /// Module rows whose live SQL Server definition differs from audit history.
    live_definition_drift: HashSet<ObjectKey>,
}

impl ChecksumMap {
    /// Creates an empty `ChecksumMap`.
    pub fn new() -> Self {
        Self {
            by_key: HashMap::new(),
            live_definition_drift: HashSet::new(),
        }
    }

    /// Reserves capacity for at least `additional` more entries.
    pub fn reserve(&mut self, additional: usize) {
        self.by_key.reserve(additional);
    }

    /// Inserts a digest for the normalized key string.
    pub fn insert_normalized(&mut self, key: &str, cs: [u8; 32]) {
        self.by_key.insert(ObjectKey::from_normalized(key), cs);
    }

    /// Hot-path insert by object key.
    pub fn insert_key(&mut self, key: &ObjectKey, cs: [u8; 32]) {
        self.by_key.insert(key.clone(), cs);
    }

    /// Returns the digest for the normalized key string, if present.
    pub fn get_normalized(&self, key: &str) -> Option<&[u8; 32]> {
        self.by_key.get(key)
    }

    /// Returns the digest for an `ObjectKey`, if present.
    pub fn get_key(&self, key: &ObjectKey) -> Option<&[u8; 32]> {
        self.by_key.get(key)
    }

    /// Marks a module as requiring a live-definition restore.
    pub fn mark_live_definition_drift(&mut self, key: &ObjectKey) {
        self.live_definition_drift.insert(key.clone());
    }

    /// Returns true when the live module body differs from the audited body.
    pub fn has_live_definition_drift(&self, key: &ObjectKey) -> bool {
        self.live_definition_drift.contains(key)
    }
}
