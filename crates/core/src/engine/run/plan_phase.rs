use crate::config::Config;
use crate::domain::Workspace;
use crate::driver::TimingConn;
use crate::error::Result;
use crate::export::MigrationPlan;
use crate::timings::PhaseTimings;

use super::planned_at::resolved_planned_at;
use super::types::{command_label, Command};

/// Inspect the database, record plan-DB timings, and diff against the workspace to
/// build the migration plan.
///
/// For mutating commands this runs inside the advisory lock (see
/// `apply_run::run_locked`) so the plan is computed from locked DB state and cannot
/// be made stale by a concurrent migrator before apply. Read-only commands call it
/// without the lock.
pub(crate) async fn plan_phase(
    cmd: Command,
    cfg: &Config,
    conn: &mut TimingConn,
    ws: &mut Workspace,
    timings: &mut PhaseTimings,
) -> Result<MigrationPlan> {
    // Mutating commands must plan from live DB state under the lock, never a
    // possibly-stale local cache.
    let db = crate::db::run_plan_db_phase(
        cfg,
        conn,
        ws,
        super::command_mutates(cmd),
        cmd == Command::RepairChecksum,
    )
    .await?;
    timings.ensure_ms = db.ensure_ms;
    timings.checksums_ms = db.checksums_ms;
    timings.inspect_ms = db.inspect_ms;
    timings.parallel_wall_ms = if db.parallel_wall_ms > 0 {
        db.parallel_wall_ms
    } else {
        db.ensure_ms.max(db.checksums_ms + db.inspect_ms)
    };
    timings.plan_db_path = db.trace.path_label().to_string();
    timings.plan_db_query_calls = db.trace.timings.query_calls;
    timings.plan_db_query_ms = db.trace.timings.query_ms;
    timings.plan_db_bootstrap = db.trace.flags.bootstrap;
    timings.plan_db_catalog_queried = db.trace.flags.catalog_queried;
    timings.plan_db_checksums_batch_ms = db.trace.timings.checksums_batch_ms;
    timings.plan_db_catalog_ms = db.trace.timings.catalog_ms;
    timings.plan_db_catalog_sql_ms = db.trace.timings.catalog_sql_ms;
    timings.plan_db_history_empty = db.trace.flags.history_empty;
    timings.plan_db_checksums_skipped = db.trace.flags.checksums_skipped;
    timings.plan_db_round_trips = db.trace.timings.round_trips;
    timings.finish_audit_ms();

    let (mut plan, diff_ms) = crate::plan::compute_diff(ws, &db.catalog, &db.checksums)?;
    plan.command = command_label(cmd).into();
    plan.planned_at = resolved_planned_at();
    timings.diff_ms = diff_ms;
    Ok(plan)
}
