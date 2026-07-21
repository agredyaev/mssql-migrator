use crate::audit;
use crate::domain::Workspace;
use crate::driver::TimingConn;
use crate::error::Result;
use crate::export::PlannedObject;

use crate::audit::HistoryRecord;

use super::history_write::apply_in_tx;
use super::result::ApplyResult;

/// Executes one object script with its history row in a single transaction —
/// for every kind, including modules: a committed module body without its
/// history row would replay (or silently drift) on the next run.
pub async fn exec_object(
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
    let recs = object_records(ws, obj);
    if apply_in_tx(conn, &body, &recs, result, &obj.normalized_key).await {
        result.applied += 1;
    }
    Ok(())
}

fn object_records(ws: &Workspace, obj: &PlannedObject) -> Vec<HistoryRecord> {
    let mut recs = vec![audit::record_event(
        &obj.normalized_key,
        obj.checksum,
        obj.git_hash(),
        obj.git_author(),
        obj.git_date(),
        "object",
        "applied",
    )];
    if obj.planned_action == crate::domain::Action::CreateObject {
        recs.extend(super::history_write::create_table_transition_records(
            ws, obj,
        ));
    }
    recs
}

pub(super) use super::script_read::read_script;
