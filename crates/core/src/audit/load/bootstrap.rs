use crate::driver::{DbClient, TimingConn};
use crate::error::Result;
use crate::sql;

use super::cache::{invalidate_audit_cache_all, mark_tables_ensured, tables_ensured};

/// Creates or upgrades audit schema tables on `client`.
pub async fn ensure_tables_on(client: &mut DbClient, db_fp: &str) -> Result<()> {
    client.exec(sql::audit::BOOTSTRAP_TABLES).await?;
    mark_tables_ensured(db_fp);
    Ok(())
}

/// Creates or upgrades audit schema tables via `conn`.
pub async fn ensure_tables(conn: &mut TimingConn, db_fp: &str) -> Result<()> {
    ensure_tables_on(conn.client_mut()?, db_fp).await
}

/// Detect audit meta tables already present in DB (process cache may be cold after e2e reset).
pub async fn probe_audit_tables_exist(conn: &mut TimingConn) -> Result<bool> {
    let rows = conn.query(crate::sql::audit::HISTORY_EXISTS, &[]).await?;
    Ok(rows.first().and_then(|r| r.get_i32(0)).unwrap_or(0) != 0)
}

/// Probes the database and updates the in-process bootstrap cache to match.
pub async fn sync_tables_ensured(conn: &mut TimingConn, db_fp: &str) -> Result<()> {
    let exists = probe_audit_tables_exist(conn).await?;
    if exists {
        mark_tables_ensured(db_fp);
    } else if tables_ensured(db_fp) {
        invalidate_audit_cache_all(db_fp);
    }
    Ok(())
}
