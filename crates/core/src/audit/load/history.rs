use crate::db::ChecksumMap;
use crate::driver::TimingConn;
use crate::error::Result;
use crate::sql;

use super::bootstrap::ensure_tables;
use super::cache::{
    cache_history_empty, history_empty_cached, history_known_nonempty, tables_ensured,
};
use super::checksum::{checksum_map_from_rows_ws, empty_checksums_from_keys_json};

pub async fn history_table_is_empty(conn: &mut TimingConn, db_fp: &str) -> Result<bool> {
    if let Some(empty) = history_empty_cached(db_fp) {
        return Ok(empty);
    }
    if !tables_ensured(db_fp) {
        ensure_tables(conn, db_fp).await?;
    }
    let rows = conn.query(sql::audit::HISTORY_EMPTY, &[]).await?;
    let has_rows = rows.first().map(history_probe_has_rows).unwrap_or(false);
    let empty = !has_rows;
    cache_history_empty(db_fp, empty);
    Ok(empty)
}

async fn history_table_is_empty_cached(conn: &mut TimingConn, db_fp: &str) -> Result<bool> {
    history_table_is_empty(conn, db_fp).await
}

pub(super) fn history_probe_has_rows(row: &crate::driver::RowData) -> bool {
    if let Some(s) = row.get_str(0) {
        return matches!(s.trim(), "1" | "true" | "True");
    }
    if let Some(n) = row.get_i32(0) {
        return n != 0;
    }
    row.get_bytes(0).is_some_and(|b| !b.is_empty() && b[0] != 0)
}

/// Latest applied/adopted digests per normalized key.
pub async fn load_checksums(
    conn: &mut TimingConn,
    db_fp: &str,
    keys_json: &str,
) -> Result<ChecksumMap> {
    if keys_json == "[]" {
        return Ok(ChecksumMap::new());
    }
    if history_known_nonempty(db_fp) {
        return load_checksums_query(conn, keys_json).await;
    }
    if history_table_is_empty_cached(conn, db_fp).await? {
        return Ok(empty_checksums_from_keys_json(keys_json));
    }
    load_checksums_query(conn, keys_json).await
}

async fn load_checksums_query(conn: &mut TimingConn, keys_json: &str) -> Result<ChecksumMap> {
    let rows = conn.query(sql::audit::LOAD_CHECKSUMS, &[keys_json]).await?;
    Ok(checksum_map_from_rows_ws(&rows))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::driver::row::{Cell, RowData};

    #[test]
    fn history_probe_parses_false_as_empty() {
        let mut row = RowData::default();
        row.cells.push(Cell::Str("0".into()));
        assert!(!history_probe_has_rows(&row));
    }

    #[test]
    fn history_probe_parses_true_as_non_empty() {
        let mut row = RowData::default();
        row.cells.push(Cell::Str("1".into()));
        assert!(history_probe_has_rows(&row));
    }

    #[test]
    fn history_probe_parses_bit_true_as_non_empty() {
        let mut row = RowData::default();
        row.cells.push(Cell::Str("1".into()));
        assert!(history_probe_has_rows(&row));
    }
}
