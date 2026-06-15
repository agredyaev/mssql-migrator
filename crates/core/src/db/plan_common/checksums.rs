use crate::audit;
use crate::db::catalog_inspect_cache;
use crate::db::plan_db_trace::PlanDbTrace;
use crate::db::state::ChecksumMap;
use crate::error::Result;
use crate::sql;

use super::conn::PlanDbConn;

pub(crate) async fn ensure_tables_plan(conn: &mut PlanDbConn<'_>, db_fp: &str) -> Result<()> {
    if audit::tables_ensured(db_fp) {
        return Ok(());
    }
    conn.exec(sql::audit::BOOTSTRAP_TABLES).await?;
    audit::mark_tables_ensured(db_fp);
    Ok(())
}

pub(crate) async fn load_checksums_plan(
    conn: &mut PlanDbConn<'_>,
    db_fp: &str,
    keys_json: &str,
) -> Result<ChecksumMap> {
    if keys_json == "[]" {
        return Ok(ChecksumMap::new());
    }
    if audit::history_known_nonempty(db_fp) {
        return load_checksums_query_plan(conn, keys_json).await;
    }
    if history_table_is_empty_plan(conn, db_fp).await? {
        return Ok(audit::empty_checksums_from_keys_json(keys_json));
    }
    load_checksums_query_plan(conn, keys_json).await
}

async fn history_table_is_empty_plan(conn: &mut PlanDbConn<'_>, db_fp: &str) -> Result<bool> {
    if let Some(empty) = audit::history_empty_cached(db_fp) {
        return Ok(empty);
    }
    if !audit::tables_ensured(db_fp) {
        ensure_tables_plan(conn, db_fp).await?;
    }
    let rows = conn.query(sql::audit::HISTORY_EMPTY, &[]).await?;
    audit::mark_tables_ensured(db_fp);
    let has_rows = rows
        .first()
        .and_then(|r| r.get_str(0))
        .is_some_and(|s| matches!(s.trim(), "1" | "true" | "True"))
        || rows
            .first()
            .and_then(|r| r.get_i32(0))
            .is_some_and(|n| n != 0);
    let empty = !has_rows;
    if !empty {
        catalog_inspect_cache::invalidate_db(db_fp);
    }
    audit::cache_history_empty(db_fp, empty);
    Ok(empty)
}

async fn load_checksums_query_plan(
    conn: &mut PlanDbConn<'_>,
    keys_json: &str,
) -> Result<ChecksumMap> {
    let rows = conn.query(sql::audit::LOAD_CHECKSUMS, &[keys_json]).await?;
    Ok(audit::checksum_map_from_rows_ws(&rows))
}

pub(crate) fn set_checksum_trace(trace: &mut PlanDbTrace, db_fp: &str, keys_json: &str) {
    if keys_json == "[]" {
        return;
    }
    if let Some(empty) = audit::history_empty_cached(db_fp) {
        trace.flags.history_empty = empty;
        if empty {
            trace.flags.checksums_skipped = true;
        }
    }
}

pub(crate) fn checksum_query_round_trips(db_fp: &str, keys_json: &str) -> i64 {
    if keys_json == "[]" {
        return 0;
    }
    if audit::history_known_nonempty(db_fp) {
        return 1;
    }
    if audit::history_known_empty(db_fp) {
        return 1;
    }
    2
}
