use crate::audit::{self, flush_history, HistoryRecord};
use crate::domain::{path_lookup_candidates, ObjectKey, ScriptKey, Workspace};
use crate::driver::TimingConn;
use crate::export::PlannedObject;

use super::result::ApplyResult;

/// A freshly created table's `.sql` already embodies all of its past transition
/// scripts; record them as applied so a later table change does not replay them.
pub(super) fn create_table_transition_records(
    ws: &Workspace,
    obj: &PlannedObject,
) -> Vec<HistoryRecord> {
    if obj.kind.as_ref() != "tables" {
        return Vec::new();
    }
    let row_id = ws.key_index(&ObjectKey::from_normalized(obj.normalized_key.as_ref()));
    let Some(paths) = ws
        .transition_path_cache
        .as_ref()
        .and_then(|m| m.get(&row_id))
    else {
        return Vec::new();
    };
    paths
        .iter()
        .map(|off| {
            let path = ws.str_at(*off);
            let cs = path_lookup_candidates(obj.database_name.as_ref(), path)
                .into_iter()
                .find_map(|key| {
                    ws.script_by_key(&ScriptKey::from_path(&key))
                        .and_then(|s| s.checksum().copied())
                })
                .unwrap_or(obj.checksum);
            audit::record_applied(
                path,
                &obj.kind,
                cs,
                obj.git_hash(),
                obj.git_author(),
                obj.git_date(),
                "migration",
            )
        })
        .collect()
}

/// Persists `records` to audit history in its own INSERT. Used for module
/// objects (CREATE OR ALTER, idempotent) and adopt records; the non-idempotent
/// wrapped/transition paths use `apply_in_tx` for full atomicity instead.
pub(super) async fn commit_history(
    conn: &mut TimingConn,
    result: &mut ApplyResult,
    records: &[HistoryRecord],
) -> bool {
    if records.is_empty() {
        return true;
    }
    match flush_history(conn, records).await {
        Ok(()) => {
            result.wrote_history = true;
            true
        }
        Err(e) => {
            result.push_error(format!("history write failed: {e}"));
            false
        }
    }
}

/// Run `body` and write `records` in ONE transaction, then commit — so an
/// interrupted run can never leave a committed script without its history row.
/// Returns `false` (and records a failure) on any step, rolling back.
pub(super) async fn apply_in_tx(
    conn: &mut TimingConn,
    body: &str,
    records: &[HistoryRecord],
    result: &mut ApplyResult,
    label: &str,
) -> bool {
    let open = format!("{}\n{}", crate::sql::apply::BEGIN_TX, body);
    if let Err(e) = conn.exec(&open).await {
        rollback(conn, result, label, &e.to_string()).await;
        return false;
    }
    if !records.is_empty() {
        if let Err(e) = flush_history(conn, records).await {
            rollback(conn, result, label, &format!("history write failed: {e}")).await;
            return false;
        }
    }
    if let Err(e) = conn.exec(crate::sql::apply::COMMIT_TX).await {
        rollback(conn, result, label, &format!("commit failed: {e}")).await;
        return false;
    }
    result.wrote_history = true;
    true
}

/// Rolls back the current transaction and records both the original error and,
/// if the rollback itself fails, the rollback failure — so the apply aborts
/// rather than running the next object inside a zombie transaction.
pub(super) async fn rollback(
    conn: &mut TimingConn,
    result: &mut ApplyResult,
    label: &str,
    err: &str,
) {
    if let Err(re) = conn.exec(crate::sql::apply::ROLLBACK).await {
        result.push_error(format!("{label}: rollback failed: {re}"));
    }
    result.push_error(format!("{label}: {err}"));
}
