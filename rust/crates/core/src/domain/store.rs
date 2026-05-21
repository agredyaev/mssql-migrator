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

/// Legacy wrapper for tests / sizeof reporting (rows + index split in [`super::Workspace`]).
#[derive(Clone, Debug, Default)]
pub struct ObjectStore {
    pub rows: Vec<ObjectRow>,
    pub key_index: HashMap<ObjectKey, u32>,
}

impl ObjectStore {
    pub fn len(&self) -> usize {
        self.rows.len()
    }

    pub fn is_empty(&self) -> bool {
        self.rows.is_empty()
    }

    pub fn row(&self, i: usize) -> &ObjectRow {
        &self.rows[i]
    }

    pub fn key_index(&self, key: &ObjectKey) -> u32 {
        self.key_index.get(key).copied().unwrap_or(0)
    }
}

pub fn finalize_sorted_entries(
    mut entries: Vec<ObjectEntry>,
) -> (
    Vec<ObjectRow>,
    HashMap<ObjectKey, u32>,
    HashMap<u64, u32>,
    Vec<ObjectEntry>,
    Vec<ObjectKey>,
) {
    let n = entries.len();
    if n == 0 {
        return (Vec::new(), HashMap::new(), HashMap::new(), entries, Vec::new());
    }
    entries.sort_by(|a, b| {
        a.staging_key
            .as_ref()
            .map(|k| k.as_str())
            .cmp(&b.staging_key.as_ref().map(|k| k.as_str()))
    });
    let mut schema_ids: HashMap<String, u16> = HashMap::new();
    let mut rows = Vec::with_capacity(n);
    let mut key_index = HashMap::with_capacity(n);
    let mut fp_index = HashMap::with_capacity(n);
    let mut object_keys = Vec::with_capacity(n);
    for (i, obj) in entries.iter().enumerate() {
        let key = obj
            .staging_key
            .as_ref()
            .expect("staging_key set before finalize");
        let schema = key.schema_part();
        let schema_id = if schema.is_empty() {
            0
        } else {
            let next = (schema_ids.len() + 1) as u16;
            *schema_ids.entry(schema.to_string()).or_insert(next)
        };
        rows.push(ObjectRow {
            schema_id,
            kind_code: kind_code(key.kind_part()),
            flags: 0,
        });
        let key = obj
            .staging_key
            .clone()
            .expect("staging_key set before finalize");
        object_keys.push(key.clone());
        let row_id = (i + 1) as u32;
        fp_index.insert(key.fingerprint(), row_id);
        key_index.insert(key, row_id);
    }
    (rows, key_index, fp_index, entries, object_keys)
}
