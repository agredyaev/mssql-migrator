use crate::audit;
use crate::domain::{path_lookup_candidates, Action, ScriptKey, Workspace};
use crate::driver::TimingConn;
use crate::error::Result;
use crate::export::{MigrationPlan, PlannedObject};

use super::result::ApplyResult;

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
    let cs = transition_checksum(ws, obj, path).unwrap_or(obj.checksum);
    let rec = audit::record_applied(
        path,
        &obj.kind,
        cs,
        obj.git_hash(),
        obj.git_author(),
        obj.git_date(),
        "migration",
    );
    // Migration body + its history row commit atomically (no replay window).
    if super::history_write::apply_in_tx(conn, &body, std::slice::from_ref(&rec), result, path)
        .await
    {
        result.applied += 1;
    }
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
