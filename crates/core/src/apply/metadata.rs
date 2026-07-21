//! Metadata-only execution for `baseline` and `repair-checksum`: both commands
//! write audit history rows and must never execute repository DDL, module
//! bodies, or transition scripts.

use crate::audit::{self, ensure_history_index, ensure_tables};
use crate::config::Config;
use crate::domain::{Action, Workspace};
use crate::driver::TimingConn;
use crate::error::{Error, Result};
use crate::export::{MigrationPlan, PlannedObject};

use super::result::ApplyResult;

/// Which audit-metadata command is executing.
#[derive(Clone, Copy, PartialEq, Eq)]
pub enum MetadataMode {
    /// Adopt existing unrecorded objects; every other action is a skip.
    Baseline,
    /// Baseline adoption plus re-baselining mismatched checksums of existing
    /// objects — the operator asserts the live state matches the repository.
    RepairChecksum,
}

/// Writes adoption/repair audit rows for the plan without executing any
/// repository SQL, then runs the shared cache-invalidation epilogue.
pub async fn execute_metadata_plan(
    cfg: &Config,
    conn: &mut TimingConn,
    ws: &Workspace,
    plan: &mut MigrationPlan,
    mode: MetadataMode,
) -> Result<ApplyResult> {
    if plan.blocked && mode != MetadataMode::RepairChecksum {
        return Err(Error::PlanBlocked);
    }
    plan.ensure_objects_materialized(ws);
    let mut result = ApplyResult::default();
    let db_fp = audit::db_fingerprint(&cfg.server, &cfg.port, &cfg.user, &cfg.database);
    ensure_tables(conn, &db_fp).await?;
    ensure_history_index(conn).await?;
    let mut recs = Vec::new();
    for obj in &plan.objects {
        result.skipped += 1;
        match obj.planned_action {
            Action::AdoptExisting => recs.push(super::objects::adopt_record(obj)),
            Action::UpdateExistingModule
            | Action::ReprocessChanged
            | Action::ReprocessChangedBlocked
            | Action::Fail
                if mode == MetadataMode::RepairChecksum =>
            {
                recs.push(repair_record(obj));
            }
            // CreateObject and the rest stay skips: an absent object cannot be
            // adopted or repaired, only `migrate` may create it.
            _ => {}
        }
    }
    super::history_write::commit_history(conn, &mut result, &recs).await;
    super::finish(cfg, conn, result).await
}

fn repair_record(obj: &PlannedObject) -> audit::HistoryRecord {
    audit::record_event(
        &obj.normalized_key,
        obj.checksum,
        obj.git_hash(),
        obj.git_author(),
        obj.git_date(),
        "object",
        "applied",
    )
}
