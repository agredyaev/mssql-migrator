use crate::audit::{flush_history, HistoryRecord};
use crate::driver::TimingConn;

use super::result::ApplyResult;
pub(super) use super::table_records::create_table_transition_records;

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
    // A script body containing its own COMMIT/ROLLBACK would close the executor
    // transaction; the history row would then autocommit and survive the failed
    // final COMMIT, permanently marking an unapplied script as applied.
    if let Err(e) = conn.exec(crate::sql::apply::ASSERT_OPEN_TX).await {
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
