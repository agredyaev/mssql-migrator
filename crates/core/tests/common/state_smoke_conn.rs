//! SQL connect + audit history probes for workflow / apply integration tests.

use migrator_core::config::Config;
use migrator_core::driver::{connect, DbClient, IoProfile, TimingConn};
use migrator_core::error::Result;

pub async fn open_conn(cfg: &Config) -> Result<TimingConn> {
    Ok(TimingConn::new(
        DbClient::Direct(connect(cfg).await?.client),
        std::sync::Arc::new(std::sync::Mutex::new(IoProfile::default())),
    ))
}

// Included per-target via `#[path]`; not every target calls every probe.
#[allow(dead_code)]
pub async fn count_audit_rows(conn: &mut TimingConn, kind: &str, event: &str) -> Result<i32> {
    let rows = conn
        .query(
            "SELECT COUNT(*) FROM azdo_deploy_meta.history WHERE kind = @p1 AND event = @p2",
            &[kind, event],
        )
        .await?;
    Ok(rows.first().and_then(|r| r.get_i32(0)).unwrap_or(0))
}
