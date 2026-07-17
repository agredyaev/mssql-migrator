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
    let Some(script) = find_transition(ws, obj, path) else {
        result.push_error(format!("{path}: transition script not found"));
        return Ok(());
    };
    let cs = script.checksum().copied().unwrap_or(obj.checksum);
    let body = match super::script_read::verified_body(script.abs_path().as_ref(), &cs, path) {
        Ok(body) => body,
        Err(msg) => {
            result.push_error(msg);
            return Ok(());
        }
    };
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

fn find_transition<'a>(
    ws: &'a Workspace,
    obj: &PlannedObject,
    path: &str,
) -> Option<crate::domain::ScriptRef<'a>> {
    path_lookup_candidates(obj.database_name.as_ref(), path)
        .into_iter()
        .find_map(|key| ws.script_by_key(&ScriptKey::from_path(&key)))
}
