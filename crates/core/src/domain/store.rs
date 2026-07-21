use std::collections::HashMap;

use super::key::ObjectKey;
use super::kind_code::kind_code;
use super::object::ObjectEntry;

/// Compact per-object metadata row stored parallel to `object_entries`.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct ObjectRow {
    /// Index into the workspace schema list (0 = no schema).
    pub schema_id: u16,
    /// Numeric kind code derived from the object kind path segment.
    pub kind_code: u8,
}

/// Output tuple produced by `finalize_sorted_entries`.
pub type FinalizeSortedResult = (
    Vec<ObjectRow>,
    HashMap<ObjectKey, u32>,
    HashMap<u64, u32>,
    Vec<ObjectEntry>,
    Vec<ObjectKey>,
);

/// Sorts entries by key, assigns schema IDs, and returns parallel row and index structures.
pub fn finalize_sorted_entries(
    mut entries: Vec<ObjectEntry>,
    mut keys: Vec<ObjectKey>,
) -> FinalizeSortedResult {
    let n = entries.len();
    assert_eq!(n, keys.len());
    if n == 0 {
        return (Vec::new(), HashMap::new(), HashMap::new(), entries, keys);
    }
    let mut pairs: Vec<(ObjectKey, ObjectEntry)> = keys.drain(..).zip(entries.drain(..)).collect();
    pairs.sort_by(|(ka, _), (kb, _)| ka.as_str().cmp(kb.as_str()));
    let mut schema_ids: HashMap<String, u16> = HashMap::new();
    let mut rows = Vec::with_capacity(n);
    let mut key_index = HashMap::with_capacity(n);
    let mut fp_index = HashMap::with_capacity(n);
    let mut object_keys = Vec::with_capacity(n);
    let mut out_entries = Vec::with_capacity(n);
    for (i, (key, obj)) in pairs.into_iter().enumerate() {
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
        });
        object_keys.push(key.clone());
        let row_id = (i + 1) as u32;
        fp_index.insert(key.fingerprint(), row_id);
        key_index.insert(key, row_id);
        out_entries.push(obj);
    }
    (rows, key_index, fp_index, out_entries, object_keys)
}
