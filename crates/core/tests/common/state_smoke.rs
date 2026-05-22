//! Smoke fixture catalog probes (`dactests` / schema `smoke`).

use migrator_core::driver::TimingConn;
use migrator_core::error::Result;

pub async fn schema_exists(conn: &mut TimingConn, schema: &str) -> Result<bool> {
    let rows = conn
        .query(
            "SELECT COUNT(*) FROM sys.schemas WHERE name = @p1",
            &[schema],
        )
        .await?;
    Ok(rows.first().and_then(|r| r.get_i32(0)).unwrap_or(0) > 0)
}

pub async fn user_table_exists(conn: &mut TimingConn, schema: &str, table: &str) -> Result<bool> {
    let rows = conn
        .query(
            r"SELECT COUNT(*)
FROM sys.tables t
INNER JOIN sys.schemas s ON s.schema_id = t.schema_id
WHERE s.name = @p1 AND t.name = @p2",
            &[schema, table],
        )
        .await?;
    Ok(rows.first().and_then(|r| r.get_i32(0)).unwrap_or(0) > 0)
}

pub async fn view_exists(conn: &mut TimingConn, schema: &str, view: &str) -> Result<bool> {
    let rows = conn
        .query(
            r"SELECT COUNT(*)
FROM sys.views v
INNER JOIN sys.schemas s ON s.schema_id = v.schema_id
WHERE s.name = @p1 AND v.name = @p2",
            &[schema, view],
        )
        .await?;
    Ok(rows.first().and_then(|r| r.get_i32(0)).unwrap_or(0) > 0)
}

pub async fn assert_smoke_objects_materialized(conn: &mut TimingConn) -> Result<()> {
    assert!(schema_exists(conn, "smoke").await?, "schema smoke missing");
    assert!(
        user_table_exists(conn, "smoke", "smoke_table").await?,
        "table smoke.smoke_table missing"
    );
    assert!(
        view_exists(conn, "smoke", "smoke_view").await?,
        "view smoke.smoke_view missing"
    );
    Ok(())
}
