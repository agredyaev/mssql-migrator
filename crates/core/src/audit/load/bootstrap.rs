use crate::driver::{DbClient, TimingConn};
use crate::error::Result;
use crate::sql;

use super::cache::{mark_tables_ensured, tables_ensured};

pub async fn ensure_tables_on(client: &mut DbClient, db_fp: &str) -> Result<()> {
    if tables_ensured(db_fp) {
        return Ok(());
    }
    client.exec(sql::audit::BOOTSTRAP_TABLES).await?;
    mark_tables_ensured(db_fp);
    Ok(())
}

pub async fn ensure_tables(conn: &mut TimingConn, db_fp: &str) -> Result<()> {
    ensure_tables_on(conn.client_mut(), db_fp).await
}

const AUDIT_HISTORY_TABLE_PROBE: &str =
    "SELECT CASE WHEN OBJECT_ID(N'azdo_deploy_meta.history', N'U') IS NOT NULL THEN 1 ELSE 0 END";

/// Detect audit meta tables already present in DB (process cache may be cold after e2e reset).
pub async fn probe_audit_tables_exist(conn: &mut TimingConn) -> Result<bool> {
    let rows = conn.query(AUDIT_HISTORY_TABLE_PROBE, &[]).await?;
    Ok(rows.first().and_then(|r| r.get_i32(0)).unwrap_or(0) != 0)
}

pub async fn sync_tables_ensured(conn: &mut TimingConn, db_fp: &str) -> Result<()> {
    if tables_ensured(db_fp) {
        return Ok(());
    }
    if probe_audit_tables_exist(conn).await? {
        mark_tables_ensured(db_fp);
    }
    Ok(())
}
