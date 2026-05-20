use std::collections::{HashMap, HashSet};
use std::sync::{Mutex, OnceLock};

use crate::db::state::ChecksumMap;
use crate::domain::ObjectKey;
use crate::driver::{DbClient, TimingConn};
use crate::error::Result;
use crate::sql;

static ENSURED_DBS: OnceLock<Mutex<HashSet<String>>> = OnceLock::new();
static HISTORY_EMPTY: OnceLock<Mutex<HashMap<String, bool>>> = OnceLock::new();

fn ensured_dbs() -> &'static Mutex<HashSet<String>> {
    ENSURED_DBS.get_or_init(|| Mutex::new(HashSet::new()))
}

fn history_nonempty_cache() -> &'static Mutex<HashSet<String>> {
    static HISTORY_NONEMPTY: OnceLock<Mutex<HashSet<String>>> = OnceLock::new();
    HISTORY_NONEMPTY.get_or_init(|| Mutex::new(HashSet::new()))
}

pub fn tables_ensured(db_fp: &str) -> bool {
    ensured_dbs().lock().unwrap().contains(db_fp)
}

pub fn mark_tables_ensured(db_fp: &str) {
    ensured_dbs().lock().unwrap().insert(db_fp.to_string());
}

pub fn mark_history_nonempty(db_fp: &str) {
    history_nonempty_cache()
        .lock()
        .unwrap()
        .insert(db_fp.to_string());
    history_empty_cache()
        .lock()
        .unwrap()
        .insert(db_fp.to_string(), false);
}

pub fn checksum_map_from_rows(rows: &[crate::driver::RowData]) -> ChecksumMap {
    let mut out = HashMap::new();
    for row in rows {
        let key = row.get_str(0).unwrap_or("");
        if let Some(arr) = parse_history_checksum(row, 1) {
            out.insert(ObjectKey::from_normalized(key), arr);
        }
    }
    out
}

pub fn looks_like_checksum_rows(rows: &[crate::driver::RowData]) -> bool {
    rows.first().is_some_and(|r| r.cells.len() == 2)
}

fn history_empty_cache() -> &'static Mutex<HashMap<String, bool>> {
    HISTORY_EMPTY.get_or_init(|| Mutex::new(HashMap::new()))
}

pub fn db_fingerprint(server: &str, database: &str) -> String {
    format!("{server}_{database}")
}

/// Drop cached history probes (after audit writes). Does not clear bootstrap cache.
pub fn invalidate_audit_cache(db_fp: &str) {
    history_empty_cache().lock().unwrap().remove(db_fp);
    history_nonempty_cache().lock().unwrap().remove(db_fp);
}

/// Full process-local audit cache drop (e.g. after DROP/CREATE test database).
pub fn invalidate_audit_cache_all(db_fp: &str) {
    invalidate_audit_cache(db_fp);
    ensured_dbs().lock().unwrap().remove(db_fp);
}

pub async fn ensure_tables_on(client: &mut DbClient, db_fp: &str) -> Result<()> {
    if ensured_dbs().lock().unwrap().contains(db_fp) {
        return Ok(());
    }
    client.exec(sql::audit::BOOTSTRAP_TABLES).await?;
    ensured_dbs().lock().unwrap().insert(db_fp.to_string());
    Ok(())
}

pub async fn ensure_tables(conn: &mut TimingConn, db_fp: &str) -> Result<()> {
    ensure_tables_on(&mut conn.inner, db_fp).await
}

async fn history_table_is_empty(conn: &mut TimingConn, db_fp: &str) -> Result<bool> {
    if let Some(empty) = history_empty_cache().lock().unwrap().get(db_fp).copied() {
        return Ok(empty);
    }
    let rows = conn.query(sql::audit::HISTORY_EMPTY, &[]).await?;
    let has_rows = rows
        .first()
        .map(history_probe_has_rows)
        .unwrap_or(false);
    let empty = !has_rows;
    history_empty_cache()
        .lock()
        .unwrap()
        .insert(db_fp.to_string(), empty);
    Ok(empty)
}

fn history_probe_has_rows(row: &crate::driver::RowData) -> bool {
    if let Some(s) = row.get_str(0) {
        return matches!(s.trim(), "1" | "true" | "True");
    }
    if let Some(n) = row.get_i32(0) {
        return n != 0;
    }
    row.get_bytes(0).is_some_and(|b| !b.is_empty() && b[0] != 0)
}

/// Latest applied/adopted digests per normalized key (mirrors Go `audit.LoadChecksums`).
pub async fn load_checksums(
    conn: &mut TimingConn,
    db_fp: &str,
    keys_json: &str,
) -> Result<ChecksumMap> {
    if keys_json == "[]" {
        return Ok(HashMap::new());
    }
    if history_nonempty_cache().lock().unwrap().contains(db_fp) {
        return Ok(load_checksums_query(conn, keys_json).await?);
    }
    if history_table_is_empty(conn, db_fp).await? {
        return Ok(HashMap::new());
    }
    load_checksums_query(conn, keys_json).await
}

async fn load_checksums_query(conn: &mut TimingConn, keys_json: &str) -> Result<ChecksumMap> {
    let rows = conn
        .query(sql::audit::LOAD_CHECKSUMS, &[keys_json])
        .await?;
    Ok(checksum_map_from_rows(&rows))
}

fn parse_history_checksum(row: &crate::driver::RowData, idx: usize) -> Option<[u8; 32]> {
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

    #[test]
    fn parse_history_checksum_hex_string() {
        let mut row = RowData::default();
        row.cells.push(Cell::Str(
            "75fdafa30d217c791047a3d8bd5f36dd62548e04a5154e758355a51525b2f973".into(),
        ));
        let sum = parse_history_checksum(&row, 0).expect("hex checksum");
        assert_eq!(hex::encode(sum), "75fdafa30d217c791047a3d8bd5f36dd62548e04a5154e758355a51525b2f973");
    }
}
