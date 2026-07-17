use crate::audit;
use crate::domain::{is_transactional_kind, Workspace};
use crate::driver::TimingConn;
use crate::error::Result;
use crate::export::PlannedObject;

use crate::audit::HistoryRecord;

use super::history_write::{apply_in_tx, commit_history};
use super::result::ApplyResult;
use super::tx::wrap_transaction;

pub async fn exec_one(
    conn: &mut TimingConn,
    ws: &Workspace,
    obj: &PlannedObject,
    result: &mut ApplyResult,
) -> Result<()> {
    let body = match read_script(ws, obj) {
        Ok(body) => body,
        Err(msg) => {
            result.push_error(msg);
            return Ok(());
        }
    };
    let sql = if is_transactional_kind(&obj.kind) {
        wrap_transaction(&body)
    } else {
        body
    };
    if let Err(e) = conn.exec(&sql).await {
        result.push_error(format!("{}: {e}", obj.normalized_key));
        return Ok(());
    }
    record_object(conn, ws, obj, result).await;
    Ok(())
}

pub async fn exec_one_wrapped(
    conn: &mut TimingConn,
    ws: &Workspace,
    obj: &PlannedObject,
    result: &mut ApplyResult,
) -> Result<()> {
    let body = match read_script(ws, obj) {
        Ok(body) => body,
        Err(msg) => {
            result.push_error(msg);
            return Ok(());
        }
    };
    // Script body + its history row commit atomically: no window where the
    // object is applied but unrecorded (which would replay it on re-run).
    let recs = object_records(ws, obj);
    if apply_in_tx(conn, &body, &recs, result, &obj.normalized_key).await {
        result.applied += 1;
    }
    Ok(())
}

fn object_records(ws: &Workspace, obj: &PlannedObject) -> Vec<HistoryRecord> {
    let mut recs = vec![audit::record_applied(
        &obj.normalized_key,
        &obj.kind,
        obj.checksum,
        obj.git_hash(),
        obj.git_author(),
        obj.git_date(),
        "object",
    )];
    if obj.planned_action == crate::domain::Action::CreateObject {
        recs.extend(super::history_write::create_table_transition_records(
            ws, obj,
        ));
    }
    recs
}

async fn record_object(
    conn: &mut TimingConn,
    ws: &Workspace,
    obj: &PlannedObject,
    result: &mut ApplyResult,
) {
    if commit_history(conn, result, &object_records(ws, obj)).await {
        result.applied += 1;
    }
}

pub(super) use super::script_read::read_script;
