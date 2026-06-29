use crate::domain::Workspace;
use crate::domain::{is_transactional_kind, Action};
use crate::driver::TimingConn;
use crate::error::Result;
use crate::export::{MigrationPlan, PlannedObject};

use super::kind::sort_tx_batch;
use super::objects_exec::{exec_one, exec_one_wrapped};
use super::result::ApplyResult;

pub async fn apply_objects(
    conn: &mut TimingConn,
    ws: &Workspace,
    plan: &MigrationPlan,
    result: &mut ApplyResult,
) -> Result<()> {
    let mut tx_batch: Vec<&PlannedObject> = Vec::new();
    let mut module_batch: Vec<&PlannedObject> = Vec::new();

    for obj in &plan.objects {
        if !should_apply(obj) {
            if should_count_skip(obj) {
                result.skipped += 1;
            }
            continue;
        }
        if is_transactional_kind(&obj.kind) {
            tx_batch.push(obj);
        } else {
            module_batch.push(obj);
        }
    }

    flush_tx(conn, ws, &mut tx_batch, result).await?;
    if result.failed > 0 {
        return Ok(());
    }
    for obj in module_batch {
        exec_one(conn, ws, obj, result).await?;
        if result.failed > 0 {
            break;
        }
    }
    Ok(())
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
        Action::SkipUnchanged
            | Action::AdoptExisting
            | Action::ReprocessChanged
            | Action::ReprocessChangedBlocked
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
        exec_one_wrapped(conn, ws, obj, result).await?;
        if result.failed > 0 {
            break;
        }
    }
    Ok(())
}
