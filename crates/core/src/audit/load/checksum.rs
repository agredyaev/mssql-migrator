use crate::db::ChecksumMap;
use crate::driver::RowData;

/// Builds a checksum map from audit history query rows.
pub fn checksum_map_from_rows(rows: &[RowData]) -> ChecksumMap {
    let mut out = ChecksumMap::new();
    for row in rows {
        let key = row.get_str(0).unwrap_or("");
        match parse_history_checksum(row, 1) {
            Some(arr) => out.insert_normalized(key, arr),
            None => tracing::warn!(
                key,
                "audit history row has an undecodable checksum; object omitted from the snapshot"
            ),
        }
    }
    out
}

/// Map wire keys via layout fingerprints (no duplicate normalized `String` in `ChecksumMap`).
pub fn checksum_map_from_rows_ws(rows: &[RowData]) -> ChecksumMap {
    use crate::domain::ObjectKey;
    let mut out = ChecksumMap::new();
    out.reserve(rows.len());
    for row in rows {
        let key = row.get_str(0).unwrap_or("");
        match parse_history_checksum(row, 1) {
            Some(arr) => out.insert_key(&ObjectKey::from_normalized(key), arr),
            None => tracing::warn!(
                key,
                "audit history row has an undecodable checksum; object omitted from the snapshot"
            ),
        }
    }
    out
}

/// Returns true when `rows` have the two-column shape expected from a checksum query.
pub fn looks_like_checksum_rows(rows: &[RowData]) -> bool {
    rows.first().is_some_and(|r| r.cells.len() == 2)
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
mod tests {
    use super::*;
    use crate::driver::row::{Cell, RowData};

    #[test]
    fn parse_history_checksum_hex_string() {
        let mut row = RowData::default();
        row.cells.push(Cell::Str(
            "75fdafa30d217c791047a3d8bd5f36dd62548e04a5154e758355a51525b2f973".into(),
        ));
        let sum = parse_history_checksum(&row, 0).expect("hex checksum");
        assert_eq!(
            hex::encode(sum),
            "75fdafa30d217c791047a3d8bd5f36dd62548e04a5154e758355a51525b2f973"
        );
    }
}
