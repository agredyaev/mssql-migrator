use std::collections::HashMap;

use super::key::ObjectKey;
use super::kind_code::kind_code;
use super::object::ObjectEntry;

#[derive(Clone, Debug, Default)]
pub struct ObjectRow {
    pub schema_id: u16,
    pub kind_code: u8,
    pub flags: u8,
}

#[derive(Clone, Debug, Default)]
pub struct ObjectStore {
    rows: Vec<ObjectRow>,
    keys: Vec<ObjectKey>,
    key_index: HashMap<ObjectKey, u32>,
}

impl ObjectStore {
    pub fn len(&self) -> usize {
        self.rows.len()
    }

    pub fn is_empty(&self) -> bool {
        self.rows.is_empty()
    }

    pub fn key(&self, i: usize) -> &ObjectKey {
        &self.keys[i]
    }

    pub fn row(&self, i: usize) -> &ObjectRow {
        &self.rows[i]
    }

    pub fn keys(&self) -> &[ObjectKey] {
        &self.keys
    }

    pub fn key_index(&self, key: &ObjectKey) -> u32 {
        self.key_index.get(key).copied().unwrap_or(0)
    }
}

pub fn finalize_sorted_entries(
    mut entries: Vec<ObjectEntry>,
) -> (ObjectStore, Vec<ObjectEntry>) {
    let n = entries.len();
    if n == 0 {
        return (ObjectStore::default(), entries);
    }
    entries.sort_by(|a, b| a.key.as_str().cmp(b.key.as_str()));
    let mut rows = Vec::with_capacity(n);
    let mut keys = Vec::with_capacity(n);
    let mut key_index = HashMap::with_capacity(n);
    for (i, obj) in entries.iter().enumerate() {
        rows.push(ObjectRow {
            schema_id: 0,
            kind_code: kind_code(obj.kind.as_ref()),
            flags: 0,
        });
        keys.push(obj.key.clone());
        key_index.insert(obj.key.clone(), (i + 1) as u32);
    }
    (
        ObjectStore {
            rows,
            keys,
            key_index,
        },
        entries,
    )
}
