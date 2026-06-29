use crate::audit;
use crate::domain::{path_lookup_candidates, Action, ScriptKey, Workspace};
use crate::driver::TimingConn;
use crate::error::Result;
use crate::export::{MigrationPlan, PlannedObject};

use super::result::ApplyResult;
use super::tx::wrap_transaction;

pub async fn apply_transitions(
    conn: &mut TimingConn,
    ws: &Workspace,
    plan: &MigrationPlan,
    result: &mut ApplyResult,
) -> Result<()> {
    for obj in &plan.objects {
        if obj.planned_action != Action::ReprocessChanged || obj.transition_paths.is_empty() {
            continue;
        }
        for path in &obj.transition_paths {
            apply_one_transition(conn, ws, obj, path, result).await?;
            if result.failed > 0 {
                return Ok(());
            }
        }
    }
    Ok(())
}

async fn apply_one_transition(
    conn: &mut TimingConn,
    ws: &Workspace,
    obj: &PlannedObject,
    path: &str,
    result: &mut ApplyResult,
) -> Result<()> {
    let Some(body) = read_transition(ws, obj, path) else {
        result.push_error(format!("{path}: transition script not found"));
        return Ok(());
    };
    let sql = wrap_transaction(&body);
    if let Err(e) = conn.exec(&sql).await {
        if let Err(re) = conn.exec(crate::sql::apply::ROLLBACK).await {
            result.push_error(format!("{path}: rollback failed: {re}"));
        }
        result.push_error(format!("{path}: {e}"));
        return Ok(());
    }
    let cs = transition_checksum(ws, obj, path).unwrap_or(obj.checksum);
    result.history.push(audit::record_applied(
        path,
        &obj.kind,
        cs,
        obj.git_hash(),
        obj.git_author(),
        obj.git_date(),
        "migration",
    ));
    result.applied += 1;
    Ok(())
}

fn read_transition(ws: &Workspace, obj: &PlannedObject, path: &str) -> Option<String> {
    for key in path_lookup_candidates(obj.database_name.as_ref(), path) {
        if let Some(script) = ws.script_by_key(&ScriptKey::from_path(&key)) {
            return std::fs::read_to_string(script.abs_path().as_ref()).ok();
        }
    }
    None
}

fn transition_checksum(ws: &Workspace, obj: &PlannedObject, path: &str) -> Option<[u8; 32]> {
    path_lookup_candidates(obj.database_name.as_ref(), path)
        .into_iter()
        .find_map(|key| {
            ws.script_by_key(&ScriptKey::from_path(&key))
                .and_then(|s| s.checksum().copied())
        })
}
