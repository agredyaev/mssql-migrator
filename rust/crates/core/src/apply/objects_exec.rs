use crate::audit;
use crate::domain::{is_transactional_kind, ObjectKey, ScriptKey, Workspace};
use crate::driver::TimingConn;
use crate::error::Result;
use crate::export::PlannedObject;

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
    push_applied(result, obj);
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
    let sql = wrap_transaction(&body);
    if let Err(e) = conn.exec(&sql).await {
        let _ = conn.exec(crate::sql::apply::ROLLBACK).await;
        result.push_error(format!("{}: {e}", obj.normalized_key));
        return Ok(());
    }
    push_applied(result, obj);
    Ok(())
}

pub fn push_applied(result: &mut ApplyResult, obj: &PlannedObject) {
    result.history.push(audit::record_applied(
        &obj.normalized_key,
        &obj.kind,
        obj.checksum,
        obj.git_hash(),
        obj.git_author(),
        obj.git_date(),
        "object",
    ));
    result.applied += 1;
}

pub fn read_script(ws: &Workspace, obj: &PlannedObject) -> Option<String> {
    let script = find_script(ws, obj)?;
    std::fs::read_to_string(script.abs_path.as_ref()).ok()
}

fn find_script<'a>(ws: &'a Workspace, obj: &PlannedObject) -> Option<&'a crate::domain::Script> {
    let path_key = ScriptKey::from_path(obj.object_path.as_ref());
    if let Some(script) = ws.scripts.get(&path_key) {
        return Some(script);
    }
    ws.scripts.values().find(|s| {
        ObjectKey::parse(s.key.as_str())
            .map(|k| k.as_str() == obj.normalized_key.as_ref())
            .unwrap_or(false)
    })
}
