use crate::audit::{self, HistoryRecord};
use crate::domain::{is_transactional_kind, Action, Workspace};
use crate::driver::TimingConn;
use crate::error::Result;
use crate::export::{MigrationPlan, PlannedObject};

use super::history_write::commit_history;
use super::kind::sort_tx_batch;
use super::modules::apply_modules;
use super::objects_exec::exec_object;
use super::result::ApplyResult;

pub async fn apply_objects(
    conn: &mut TimingConn,
    ws: &Workspace,
    plan: &MigrationPlan,
    result: &mut ApplyResult,
) -> Result<()> {
    let mut tx_batch: Vec<&PlannedObject> = Vec::new();
    let mut module_batch: Vec<&PlannedObject> = Vec::new();
    let mut adopt_recs: Vec<HistoryRecord> = Vec::new();

    for obj in &plan.objects {
        match obj.planned_action {
            Action::AdoptExisting => {
                result.skipped += 1;
                adopt_recs.push(adopt_record(obj));
            }
            // A changed non-table object (index/type/sequence/synonym) has no
            // transition mechanism → fail closed instead of silently skipping.
            // Tables are excluded: no-transitions tables are blocked at plan
            // time, and a table whose transitions are all applied reaches here
            // empty as a legitimate no-op skip.
            Action::ReprocessChanged
                if obj.transition_paths.is_empty() && obj.kind.as_ref() != "tables" =>
            {
                result.push_error(format!(
                    "{}: changed object cannot be auto-applied (no transition script)",
                    obj.normalized_key
                ));
                return Ok(());
            }
            _ if should_apply(obj) => {
                if is_transactional_kind(&obj.kind) {
                    tx_batch.push(obj);
                } else {
                    module_batch.push(obj);
                }
            }
            _ if should_count_skip(obj) => result.skipped += 1,
            _ => {}
        }
    }

    if !commit_history(conn, result, &adopt_recs).await {
        return Ok(());
    }
    flush_tx(conn, ws, &mut tx_batch, result).await?;
    if result.failed > 0 {
        return Ok(());
    }
    sort_tx_batch(&mut module_batch);
    apply_modules(conn, ws, module_batch, result).await?;
    Ok(())
}

pub(super) fn adopt_record(obj: &PlannedObject) -> HistoryRecord {
    audit::record_event(
        &obj.normalized_key,
        obj.checksum,
        obj.git_hash(),
        obj.git_author(),
        obj.git_date(),
        "object",
        "adopted",
    )
}

fn should_apply(obj: &PlannedObject) -> bool {
    matches!(
        obj.planned_action,
        Action::CreateObject | Action::UpdateExistingModule
    )
}

fn should_count_skip(obj: &PlannedObject) -> bool {
    matches!(
        obj.planned_action,
        Action::SkipUnchanged | Action::ReprocessChanged | Action::ReprocessChangedBlocked
    )
}

async fn flush_tx(
    conn: &mut TimingConn,
    ws: &Workspace,
    batch: &mut Vec<&PlannedObject>,
    result: &mut ApplyResult,
) -> Result<()> {
    if batch.is_empty() {
        return Ok(());
    }
    sort_tx_batch(batch);
    for obj in batch.drain(..) {
        exec_object(conn, ws, obj, result).await?;
        if result.failed > 0 {
            break;
        }
    }
    Ok(())
}
