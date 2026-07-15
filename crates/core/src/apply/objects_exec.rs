use crate::audit;
use crate::domain::{
    is_transactional_kind, path_lookup_candidates, ObjectKey, ScriptKey, Workspace,
};
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
    let Some(body) = read_script(ws, obj) else {
        result.push_error(format!("{}: script not found", obj.normalized_key));
        return Ok(());
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
    let Some(body) = read_script(ws, obj) else {
        result.push_error(format!("{}: script not found", obj.normalized_key));
        return Ok(());
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

pub fn read_script(ws: &Workspace, obj: &PlannedObject) -> Option<String> {
    let script = find_script(ws, obj)?;
    std::fs::read_to_string(script.abs_path().as_ref()).ok()
}

fn find_script<'a>(ws: &'a Workspace, obj: &PlannedObject) -> Option<crate::domain::ScriptRef<'a>> {
    for path in path_lookup_candidates(obj.database_name.as_ref(), obj.object_path.as_ref()) {
        if let Some(script) = ws.script_by_key(&ScriptKey::from_path(&path)) {
            return Some(script);
        }
    }
    ws.scripts_iter().find(|s| {
        ObjectKey::parse(s.path_str())
            .map(|k| k.as_str() == obj.normalized_key.as_ref())
            .unwrap_or(false)
    })
}
