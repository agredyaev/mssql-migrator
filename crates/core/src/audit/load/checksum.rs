use crate::db::ChecksumMap;
use crate::domain::ObjectKey;
use crate::driver::RowData;
use crate::error::{Error, Result};

/// Map wire keys via layout fingerprints (no duplicate normalized `String` in `ChecksumMap`).
pub fn checksum_map_from_rows_ws(rows: &[RowData], allow_repair: bool) -> Result<ChecksumMap> {
    let mut out = ChecksumMap::new();
    out.reserve(rows.len());
    for row in rows {
        let key = row.get_str(0).unwrap_or("");
        match parse_history_checksum(row, 1) {
            Some(arr) => {
                let key = ObjectKey::from_normalized(key);
                out.insert_key(&key, arr);
                if arr != [0; 32] && row.get_i32(2).is_some_and(|n| n != 0) {
                    out.mark_live_definition_drift(&key);
                }
            }
            None if allow_repair => {
                tracing::warn!(
                    key,
                    "audit history checksum is undecodable; repair-checksum will replace it"
                );
                out.insert_key(&ObjectKey::from_normalized(key), [0; 32]);
            }
            None => return Err(corrupt_checksum(key)),
        }
    }
    Ok(out)
}

fn corrupt_checksum(key: &str) -> Error {
    Error::Checksum(format!(
        "audit history checksum for {key} is undecodable; run repair-checksum"
    ))
}

/// Returns true when `rows` have the checksum-query key and digest columns.
pub fn looks_like_checksum_rows(rows: &[RowData]) -> bool {
    rows.first().is_some_and(|r| r.cells.len() >= 2)
}

/// One zero digest per key when audit history is empty.
pub fn empty_checksums_from_keys_json(keys_json: &str) -> ChecksumMap {
    use crate::domain::ObjectKey;
    let keys: Vec<String> = serde_json::from_str(keys_json).unwrap_or_default();
    let mut out = ChecksumMap::new();
    out.reserve(keys.len());
    for k in keys {
        out.insert_key(&ObjectKey::from_normalized(&k), [0; 32]);
    }
    out
}

pub(super) fn parse_history_checksum(row: &RowData, idx: usize) -> Option<[u8; 32]> {
    if let Some(bytes) = row.get_bytes(idx) {
        if bytes.len() == 32 {
            let mut arr = [0u8; 32];
            arr.copy_from_slice(bytes);
            return Some(arr);
        }
    }
    let s = row.get_str(idx)?.trim();
    if s.is_empty() {
        return Some([0; 32]);
    }
    let bytes = hex::decode(s).ok()?;
    if bytes.len() != 32 {
        return None;
    }
    let mut arr = [0u8; 32];
    arr.copy_from_slice(&bytes);
    Some(arr)
}

#[cfg(test)]
#[path = "../../tests/audit_checksum_test.rs"]
mod tests;
