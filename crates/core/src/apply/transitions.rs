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
        let last = obj.transition_paths.len() - 1;
        for (i, path) in obj.transition_paths.iter().enumerate() {
            apply_one_transition(conn, ws, obj, path, i == last, result).await?;
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
    is_last: bool,
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
    // Audit provenance belongs to the transition SCRIPT's commit, not the
    // parent table file's.
    let mut recs = vec![audit::record_event(
        path,
        cs,
        script.git_hash().as_ref(),
        script.git_author().as_ref(),
        script.git_date().as_ref(),
        "migration",
        "applied",
    )];
    if is_last {
        // The final pending transition brings the live table up to the current
        // repository definition: advance the table's object baseline in the
        // SAME transaction, or every later plan stays ReprocessChanged forever.
        recs.push(audit::record_event(
            &obj.normalized_key,
            obj.checksum,
            obj.git_hash(),
            obj.git_author(),
            obj.git_date(),
            "object",
            "applied",
        ));
    }
    // Migration body + its history rows commit atomically (no replay window).
    if super::history_write::apply_in_tx(conn, &body, &recs, result, path).await {
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
