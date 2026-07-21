use serde::Serialize;

use crate::driver::TimingConn;
use crate::error::Result;
use crate::sql;

/// Audit record written to the history index after an object is applied.
#[derive(Clone, Debug, Serialize)]
pub struct HistoryRecord {
    /// Normalized object key identifying the migration target.
    pub normalized_key: String,
    /// Object kind label (e.g. `"procedure"`, `"view"`).
    pub kind: String,
    /// Hex-encoded SHA-256 checksum of the applied script.
    pub checksum: String,
    /// Git commit hash of the applied revision.
    pub git_hash: String,
    /// Git author string from the applied commit.
    pub git_author: String,
    /// Git commit date string from the applied revision.
    pub git_date: String,
    /// Lifecycle event label (e.g. `"applied"`).
    pub event: String,
    /// Error message captured when the apply failed; empty on success.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub error_text: String,
}

/// Builds a `HistoryRecord` for a lifecycle `event` (e.g. `"applied"` after a
/// successful apply, `"adopted"` when a pre-existing object is taken as
/// baseline so later edits are detected as changes).
pub fn record_event(
    key: &str,
    checksum: [u8; 32],
    git_hash: &str,
    git_author: &str,
    git_date: &str,
    record_kind: &str,
    event: &str,
) -> HistoryRecord {
    HistoryRecord {
        normalized_key: key.to_string(),
        kind: record_kind.to_string(),
        checksum: hex::encode(checksum),
        git_hash: git_hash.to_string(),
        git_author: git_author.to_string(),
        git_date: git_date.to_string(),
        event: event.to_string(),
        error_text: String::new(),
    }
}

/// Creates the audit history index table in the database if it does not exist.
pub async fn ensure_history_index(conn: &mut TimingConn) -> Result<()> {
    conn.exec(sql::audit::BOOTSTRAP_INDEX).await
}

/// Inserts `records` into the audit history index in a single batch.
pub async fn flush_history(conn: &mut TimingConn, records: &[HistoryRecord]) -> Result<()> {
    if records.is_empty() {
        return Ok(());
    }
    let payload =
        serde_json::to_string(records).map_err(|e| crate::error::Error::Other(e.into()))?;
    conn.query(sql::audit::INSERT_HISTORY, &[payload.as_str()])
        .await?;
    Ok(())
}
