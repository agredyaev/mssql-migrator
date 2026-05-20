mod kind;
mod objects;
mod objects_exec;
mod result;
mod schemas;
mod transitions;
mod tx;

pub use result::ApplyResult;

use crate::audit::{self, ensure_history_index, ensure_tables, flush_history, invalidate_audit_cache};
use crate::config::Config;
use crate::db;
use crate::domain::Workspace;
use crate::driver::TimingConn;
use crate::error::{Error, Result};
use crate::export::MigrationPlan;

pub async fn execute_plan(
    cfg: &Config,
    conn: &mut TimingConn,
    ws: &Workspace,
    plan: &MigrationPlan,
) -> Result<ApplyResult> {
    if plan.blocked {
        return Err(Error::PlanBlocked);
    }
    let mut result = ApplyResult::default();
    let db_fp = audit::db_fingerprint(&cfg.server, &cfg.database);
    ensure_tables(conn, &db_fp).await?;
    schemas::apply_schemas(conn, plan, &mut result).await?;
    if result.failed > 0 {
        return finish(cfg, conn, result).await;
    }
    objects::apply_objects(conn, ws, plan, &mut result).await?;
    transitions::apply_transitions(conn, ws, plan, &mut result).await?;
    finish(cfg, conn, result).await
}

async fn finish(cfg: &Config, conn: &mut TimingConn, mut result: ApplyResult) -> Result<ApplyResult> {
    let db_fp = audit::db_fingerprint(&cfg.server, &cfg.database);
    if !result.history.is_empty() {
        ensure_history_index(conn).await?;
        let records = std::mem::take(&mut result.history);
        flush_history(conn, &records).await?;
        invalidate_audit_cache(&db_fp);
        audit::mark_history_nonempty(&db_fp);
    }
    if result.failed > 0 {
        return Err(Error::Sql(result.errors.join("; ")));
    }
    if result.applied > 0 {
        db::invalidate(conn).await?;
        let l1 = crate::cache::l1::L1Cache::new(&cfg.l1_cache_dir);
        let fp = format!("{}_{}", cfg.server, cfg.database);
        let _ = l1.invalidate_all(&fp);
    }
    Ok(result)
}
