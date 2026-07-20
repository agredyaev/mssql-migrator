use crate::error::Result;

use super::conn::PlanDbConn;

/// Read-only probe: does the audit history table exist at all? Lets
/// plan/validate treat a missing table as "no baselines" without running DDL.
pub(crate) async fn history_table_exists(conn: &mut PlanDbConn<'_>) -> Result<bool> {
    let rows = conn.query(crate::sql::audit::HISTORY_EXISTS, &[]).await?;
    Ok(rows.first().and_then(|r| r.get_i32(0)).unwrap_or(0) != 0)
}
