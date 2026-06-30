//! Transactional schema migration and object apply execution.
//!
//! ### Purpose
//! Executes planned schema DDL migrations, dynamic structural objects (views, procedures, functions),
//! and layout transition scripts against the target database catalog.
//!
//! ### Architectural Context
//! - **Inputs**: `Config` env specifications, target database connection, `Workspace` schema definition, and `MigrationPlan` listing resolved actions.
//! - **Outputs**: `ApplyResult` mapping applied/failed migration counts and logged execution records.
//! - **Boundaries**: All dynamic schema migrations are executed within explicit database transactions.
//!
//! ### Nominal Flow
//! 1. Verify that the planning phase is not blocked.
//! 2. Ensure targeted database structural tables exist (`audit::ensure_tables`).
//! 3. Apply schema migrations sequentially (`schemas::apply_schemas`).
//! 4. Apply non-schema structural objects (`objects::apply_objects`).
//! 5. Execute state layout transitions (`transitions::apply_transitions`).
//! 6. Flush generated history logs and invalidate downstream memory caches on completion.
//!
//! ### Off-Nominal & Failure Containment
//! - **Execution Exceptions**: If any query execution fails, the active database transaction is rolled back immediately, halting subsequent steps, flushing failure messages to stderr, and returning `Error::Sql`.
//! - **Blocked Plans**: Blocked plans automatically fail-closed with `Error::PlanBlocked` before hitting the database.

mod kind;
mod objects;
mod objects_exec;
mod result;
mod schemas;
mod transitions;
mod tx;

pub use result::ApplyResult;

use crate::audit::{
    self, ensure_history_index, ensure_tables, flush_history, invalidate_audit_cache,
};
use crate::config::Config;
use crate::db;
use crate::domain::Workspace;
use crate::driver::TimingConn;
use crate::error::{Error, Result};
use crate::export::MigrationPlan;

/// Executes all planned schema mutations against the target database and returns apply statistics.
pub async fn execute_plan(
    cfg: &Config,
    conn: &mut TimingConn,
    ws: &Workspace,
    plan: &mut MigrationPlan,
) -> Result<ApplyResult> {
    if plan.blocked {
        return Err(Error::PlanBlocked);
    }
    plan.ensure_objects_materialized(ws);
    let mut result = ApplyResult::default();
    let db_fp = audit::db_fingerprint(&cfg.server, &cfg.database);
    ensure_tables(conn, &db_fp).await?;
    schemas::apply_schemas(conn, plan, &mut result).await?;
    if result.failed > 0 {
        return finish(cfg, conn, result).await;
    }
    objects::apply_objects(conn, ws, plan, &mut result).await?;
    if result.failed > 0 {
        return finish(cfg, conn, result).await;
    }
    transitions::apply_transitions(conn, ws, plan, &mut result).await?;
    finish(cfg, conn, result).await
}

async fn finish(
    cfg: &Config,
    conn: &mut TimingConn,
    mut result: ApplyResult,
) -> Result<ApplyResult> {
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
