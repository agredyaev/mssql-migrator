//! Smoke fixture catalog probes (`dactests` / schema `smoke`).

use migrator_core::config::Config;
use migrator_core::driver::{connect, DbClient, IoProfile, TimingConn};
use migrator_core::error::Result;

pub async fn open_conn(cfg: &Config) -> Result<TimingConn> {
    Ok(TimingConn::new(
        DbClient::Direct(connect(cfg).await?.client),
        std::sync::Arc::new(std::sync::Mutex::new(IoProfile::default())),
        0,
    ))
}

async fn schema_exists(conn: &mut TimingConn, schema: &str) -> Result<bool> {
    let rows = conn
        .query(
            "SELECT COUNT(*) FROM sys.schemas WHERE name = @p1",
            &[schema],
        )
        .await?;
    Ok(rows.first().and_then(|r| r.get_i32(0)).unwrap_or(0) > 0)
}

async fn user_table_exists(conn: &mut TimingConn, schema: &str, table: &str) -> Result<bool> {
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

pub async fn count_audit_rows(conn: &mut TimingConn, kind: &str, event: &str) -> Result<i32> {
    let rows = conn
        .query(
            "SELECT COUNT(*) FROM azdo_deploy_meta.history WHERE kind = @p1 AND event = @p2",
            &[kind, event],
        )
        .await?;
    Ok(rows.first().and_then(|r| r.get_i32(0)).unwrap_or(0))
}

pub async fn assert_smoke_objects_materialized(conn: &mut TimingConn) -> Result<()> {
    assert!(schema_exists(conn, "smoke").await?, "schema smoke missing");
    assert!(
        user_table_exists(conn, "smoke", "smoke_table").await?,
        "table smoke.smoke_table missing"
    );
    let rows = conn
        .query(
            r"SELECT COUNT(*)
FROM sys.views v
INNER JOIN sys.schemas s ON s.schema_id = v.schema_id
WHERE s.name = 'smoke' AND v.name = 'smoke_view'",
            &[],
        )
        .await?;
    assert!(
        rows.first().and_then(|r| r.get_i32(0)).unwrap_or(0) > 0,
        "view smoke.smoke_view missing"
    );
    Ok(())
}
