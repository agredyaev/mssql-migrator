use crate::driver::{DbClient, TimingConn};
use crate::error::Result;
use crate::sql;

use super::cache::{invalidate_audit_cache_all, mark_tables_ensured, tables_ensured};

pub async fn ensure_tables_on(client: &mut DbClient, db_fp: &str) -> Result<()> {
    if tables_ensured(db_fp) {
        return Ok(());
    }
    client.exec(sql::audit::BOOTSTRAP_TABLES).await?;
    mark_tables_ensured(db_fp);
    Ok(())
}

pub async fn ensure_tables(conn: &mut TimingConn, db_fp: &str) -> Result<()> {
    ensure_tables_on(conn.client_mut()?, db_fp).await
}

const AUDIT_HISTORY_TABLE_PROBE: &str =
    "SELECT CASE WHEN OBJECT_ID(N'azdo_deploy_meta.history', N'U') IS NOT NULL THEN 1 ELSE 0 END";

/// Detect audit meta tables already present in DB (process cache may be cold after e2e reset).
pub async fn probe_audit_tables_exist(conn: &mut TimingConn) -> Result<bool> {
    let rows = conn.query(AUDIT_HISTORY_TABLE_PROBE, &[]).await?;
    Ok(rows.first().and_then(|r| r.get_i32(0)).unwrap_or(0) != 0)
}

pub async fn sync_tables_ensured(conn: &mut TimingConn, db_fp: &str) -> Result<()> {
    let exists = probe_audit_tables_exist(conn).await?;
    if exists {
        mark_tables_ensured(db_fp);
    } else if tables_ensured(db_fp) {
        invalidate_audit_cache_all(db_fp);
    }
    Ok(())
}
