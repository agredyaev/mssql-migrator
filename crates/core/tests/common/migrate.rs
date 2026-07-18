//! Apply smoke for e2e (plan pipeline + apply, same post-apply persistence as engine).

use std::sync::{Arc, Mutex};

use migrator_core::apply::execute_plan;
use migrator_core::audit::{db_fingerprint, ensure_tables, invalidate_audit_cache};
use migrator_core::config::Config;
use migrator_core::domain::Workspace;
use migrator_core::driver::{connect, DbClient, IoProfile, TimingConn};
use migrator_core::error::{Error, Result};
use migrator_core::scan;

pub struct ApplySmokeOut {
    pub applied: i32,
    pub failed: i32,
    pub skipped: i32,
    pub audit_object_rows: i32,
    pub audit_migration_rows: i32,
    pub catalog_meta_rows: i32,
    pub catalog_cache_rows: i32,
}

#[derive(Debug)]
pub struct AuditDbSnapshot {
    pub audit_object_rows: i32,
    pub audit_migration_rows: i32,
    pub catalog_meta_rows: i32,
    pub catalog_cache_rows: i32,
}

/// Fast path for blocked/ddl e2e: skip cold apply when smoke baseline is already in SQL.
pub async fn ensure_smoke_baseline(cfg: &Config) -> Result<()> {
    let io_arc = Arc::new(Mutex::new(IoProfile::default()));
    let mut conn = TimingConn::new(DbClient::Direct(connect(cfg).await?.client), io_arc, 0);
    if smoke_baseline_ready(cfg, &mut conn).await? {
        return Ok(());
    }
    run_apply_smoke(cfg).await.map(|_| ())
}

pub async fn smoke_baseline_ready(cfg: &Config, conn: &mut TimingConn) -> Result<bool> {
    let rows = conn
        .query(
            r"SELECT
    (SELECT COUNT(*) FROM sys.schemas WHERE name = N'smoke') AS schema_ok,
    (SELECT COUNT(*)
     FROM sys.tables t
     INNER JOIN sys.schemas s ON s.schema_id = t.schema_id
     WHERE s.name = N'smoke' AND t.name = N'smoke_table') AS table_ok,
    (SELECT COUNT(*)
     FROM sys.views v
     INNER JOIN sys.schemas s ON s.schema_id = v.schema_id
     WHERE s.name = N'smoke' AND v.name = N'smoke_view') AS view_ok,
    COL_LENGTH(N'smoke.smoke_table', N'added_at') AS added_at_col,
    (SELECT COUNT(*) FROM azdo_deploy_meta.history WHERE kind = N'object') AS object_rows,
    (SELECT COUNT(*) FROM azdo_deploy_meta.history WHERE kind = N'migration') AS migration_rows,
    (SELECT COUNT(*) FROM azdo_deploy_meta.catalog_meta) AS meta_rows,
    (SELECT COUNT(*) FROM azdo_deploy_meta.catalog_cache) AS cache_rows",
            &[],
        )
        .await?;
    let row = rows
        .first()
        .ok_or_else(|| Error::Other(anyhow::anyhow!("smoke baseline probe returned no rows")))?;
    if row.get_i32(0).unwrap_or(0) == 0 {
        return Ok(false);
    }
    if row.get_i32(1).unwrap_or(0) == 0 {
        return Ok(false);
    }
    if row.get_i32(2).unwrap_or(0) == 0 {
        return Ok(false);
    }
    if row.get_i32(3).is_some() {
        return Ok(false);
    }
    if row.get_i32(4).unwrap_or(0) < 6 {
        return Ok(false);
    }
    if row.get_i32(5).unwrap_or(0) != 0 {
        return Ok(false);
    }
    if cfg.catalog_cache() {
        if row.get_i32(6).unwrap_or(0) < 1 {
            return Err(Error::Other(anyhow::anyhow!(
                "catalog_meta empty with RMIG_CATALOG_CACHE enabled"
            )));
        }
        if row.get_i32(7).unwrap_or(0) < 1 {
            return Err(Error::Other(anyhow::anyhow!(
                "catalog_cache empty with RMIG_CATALOG_CACHE enabled"
            )));
        }
    }
    Ok(true)
}

pub async fn run_apply_smoke(cfg: &Config) -> Result<ApplySmokeOut> {
    let mut ws = Workspace::default();
    scan::populate(&mut ws, &cfg.sql_root, cfg.skip_git()).await?;

    let db_fp = db_fingerprint(&cfg.server, &cfg.port, &cfg.user, &cfg.database);
    invalidate_audit_cache(&db_fp);

    let l1 = migrator_core::cache::l1::L1Cache::new(&cfg.l1_cache_dir);
    let _ = l1.invalidate_all(&db_fp);

    let io_arc = Arc::new(Mutex::new(IoProfile::default()));
    let mut conn = TimingConn::new(DbClient::Direct(connect(cfg).await?.client), io_arc, 0);
    ensure_tables(&mut conn, &db_fp).await?;

    let db = migrator_core::db::run_plan_db_phase(cfg, &mut conn, &ws, false, false).await?;
    let (mut plan, _) = migrator_core::plan::compute_diff(&mut ws, &db.catalog, &db.checksums)?;
    plan.ensure_objects_materialized(&ws);

    let apply = execute_plan(cfg, &mut conn, &ws, &mut plan).await?;
    if apply.failed == 0 && apply.applied > 0 {
        migrator_core::db::save_workspace_snapshot(
            &mut conn,
            &ws.layout_digest,
            &ws,
            cfg.catalog_cache(),
        )
        .await?;
    }

    let snap = snapshot_audit_db(&mut conn).await?;

    Ok(ApplySmokeOut {
        applied: i32::try_from(apply.applied).unwrap_or(i32::MAX),
        failed: i32::try_from(apply.failed).unwrap_or(i32::MAX),
        skipped: i32::try_from(apply.skipped).unwrap_or(i32::MAX),
        audit_object_rows: snap.audit_object_rows,
        audit_migration_rows: snap.audit_migration_rows,
        catalog_meta_rows: snap.catalog_meta_rows,
        catalog_cache_rows: snap.catalog_cache_rows,
    })
}

pub async fn snapshot_audit_db(conn: &mut TimingConn) -> Result<AuditDbSnapshot> {
    Ok(AuditDbSnapshot {
        audit_object_rows: count_audit_rows(conn, "object").await?,
        audit_migration_rows: count_audit_rows(conn, "migration").await?,
        catalog_meta_rows: count_table_rows(conn, "azdo_deploy_meta.catalog_meta").await?,
        catalog_cache_rows: count_table_rows(conn, "azdo_deploy_meta.catalog_cache").await?,
    })
}

pub async fn count_audit_rows(conn: &mut TimingConn, kind: &str) -> Result<i32> {
    let rows = conn
        .query(
            "SELECT COUNT(*) FROM azdo_deploy_meta.history WHERE kind = @p1",
            &[kind],
        )
        .await?;
    Ok(rows.first().and_then(|r| r.get_i32(0)).unwrap_or(0))
}

pub async fn migration_history_keys(conn: &mut TimingConn) -> Result<Vec<String>> {
    let rows = conn
        .query(
            "SELECT normalized_key FROM azdo_deploy_meta.history \
             WHERE kind = 'migration' AND event = 'applied' ORDER BY id",
            &[],
        )
        .await?;
    Ok(rows
        .iter()
        .filter_map(|r| r.get_str(0).map(str::to_string))
        .collect())
}

async fn count_table_rows(conn: &mut TimingConn, table: &str) -> Result<i32> {
    let sql = format!("SELECT COUNT(*) FROM {table}");
    let rows = conn.query(&sql, &[]).await?;
    Ok(rows.first().and_then(|r| r.get_i32(0)).unwrap_or(0))
}

pub fn assert_catalog_cache_when_enabled(cfg: &Config, snap: &AuditDbSnapshot) -> Result<()> {
    if !cfg.catalog_cache() {
        return Ok(());
    }
    if snap.catalog_meta_rows < 1 {
        return Err(Error::Other(anyhow::anyhow!(
            "catalog_meta empty with RMIG_CATALOG_CACHE enabled (rows={})",
            snap.catalog_meta_rows
        )));
    }
    if snap.catalog_cache_rows < 1 {
        return Err(Error::Other(anyhow::anyhow!(
            "catalog_cache empty with RMIG_CATALOG_CACHE enabled (rows={})",
            snap.catalog_cache_rows
        )));
    }
    Ok(())
}

pub fn assert_migration_history(snap: &AuditDbSnapshot, keys: &[String]) -> Result<()> {
    if snap.audit_migration_rows < 1 {
        return Err(Error::Other(anyhow::anyhow!(
            "expected migration history rows, got {}",
            snap.audit_migration_rows
        )));
    }
    let ok = keys
        .iter()
        .any(|k| k.contains("_migrations/") && k.ends_with(".sql"));
    if !ok {
        return Err(Error::Other(anyhow::anyhow!(
            "migration history keys missing _migrations/*.sql: {keys:?}"
        )));
    }
    Ok(())
}

/// Reconnect and run full cold-apply DB invariants (smoke objects, audit, catalog).
pub async fn verify_cold_apply_report(cfg: &Config, out: &ApplySmokeOut) -> Result<()> {
    let mut conn = TimingConn::new(
        DbClient::Direct(connect(cfg).await?.client),
        Arc::new(Mutex::new(IoProfile::default())),
        0,
    );
    let snap = snapshot_audit_db(&mut conn).await?;
    if snap.audit_object_rows != out.audit_object_rows
        || snap.audit_migration_rows != out.audit_migration_rows
        || snap.catalog_meta_rows != out.catalog_meta_rows
        || snap.catalog_cache_rows != out.catalog_cache_rows
    {
        return Err(Error::Other(anyhow::anyhow!(
            "audit snapshot mismatch: db={snap:?} report object={} migration={} meta={} cache={}",
            out.audit_object_rows,
            out.audit_migration_rows,
            out.catalog_meta_rows,
            out.catalog_cache_rows,
        )));
    }
    super::e2e_verify::verify_cold_apply(cfg, &mut conn, &snap).await
}
