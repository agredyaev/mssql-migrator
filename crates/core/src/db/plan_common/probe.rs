use crate::error::Result;

use super::conn::PlanDbConn;

/// Read-only probe: does the audit history table exist at all? Lets
/// plan/validate treat a missing table as "no baselines" without running DDL.
pub(crate) async fn history_table_exists(conn: &mut PlanDbConn<'_>) -> Result<bool> {
    const PROBE: &str =
        "SELECT CASE WHEN OBJECT_ID('azdo_deploy_meta.history') IS NULL THEN 0 ELSE 1 END";
    let rows = conn.query(PROBE, &[]).await?;
    Ok(rows.first().and_then(|r| r.get_i32(0)).unwrap_or(0) != 0)
}
