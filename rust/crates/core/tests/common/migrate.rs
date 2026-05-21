//! Apply smoke for e2e (mirrors Go runApplySmoke: plan pipeline + apply.Execute).

use std::sync::{Arc, Mutex};

use migrator_core::apply::execute_plan;
use migrator_core::audit::{db_fingerprint, ensure_tables, invalidate_audit_cache};
use migrator_core::config::Config;
use migrator_core::domain::Workspace;
use migrator_core::driver::{connect, DbClient, IoProfile, TimingConn};
use migrator_core::error::Result;
use migrator_core::scan;

pub struct ApplySmokeOut {
    pub applied: i32,
    pub failed: i32,
    pub skipped: i32,
    pub audit_object_rows: i32,
}

pub async fn run_apply_smoke(cfg: &Config) -> Result<ApplySmokeOut> {
    let mut ws = Workspace::default();
    scan::populate(&mut ws, &cfg.sql_root, cfg.skip_git).await?;

    let db_fp = db_fingerprint(&cfg.server, &cfg.database);
    invalidate_audit_cache(&db_fp);

    let fp = format!("{}_{}", cfg.server, cfg.database);
    let l1 = migrator_core::cache::l1::L1Cache::new(&cfg.l1_cache_dir);
    let _ = l1.invalidate_all(&fp);

    let io_arc = Arc::new(Mutex::new(IoProfile::default()));
    let mut conn = TimingConn::new(DbClient::Direct(connect(cfg).await?.client), io_arc, 0);
    let db_fp = db_fingerprint(&cfg.server, &cfg.database);
    ensure_tables(&mut conn, &db_fp).await?;

    let db = migrator_core::db::run_plan_db_phase(cfg, &mut conn, &ws).await?;
    let (mut plan, _) = migrator_core::plan::compute_diff(&mut ws, &db.catalog, &db.checksums)?;
    plan.ensure_objects_materialized(&ws);

    let apply = execute_plan(cfg, &mut conn, &ws, &mut plan).await?;
    let audit_object_rows = count_audit_object_rows(&mut conn).await?;

    Ok(ApplySmokeOut {
        applied: i32::try_from(apply.applied).unwrap_or(i32::MAX),
        failed: i32::try_from(apply.failed).unwrap_or(i32::MAX),
        skipped: i32::try_from(apply.skipped).unwrap_or(i32::MAX),
        audit_object_rows,
    })
}

async fn count_audit_object_rows(conn: &mut TimingConn) -> Result<i32> {
    let rows = conn
        .query(
            "SELECT COUNT(*) FROM azdo_deploy_meta.history WHERE kind = 'object'",
            &[],
        )
        .await?;
    Ok(rows.first().and_then(|r| r.get_i32(0)).unwrap_or(0))
}
