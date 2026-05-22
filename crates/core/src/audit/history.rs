use serde::Serialize;

use crate::driver::TimingConn;
use crate::error::Result;
use crate::sql;

#[derive(Clone, Debug, Serialize)]
pub struct HistoryRecord {
    pub normalized_key: String,
    pub kind: String,
    pub checksum: String,
    pub git_hash: String,
    pub git_author: String,
    pub git_date: String,
    pub event: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub error_text: String,
}

pub fn record_applied(
    key: &str,
    _object_kind: &str,
    checksum: [u8; 32],
    git_hash: &str,
    git_author: &str,
    git_date: &str,
    record_kind: &str,
) -> HistoryRecord {
    HistoryRecord {
        normalized_key: key.to_string(),
        kind: record_kind.to_string(),
        checksum: hex::encode(checksum),
        git_hash: git_hash.to_string(),
        git_author: git_author.to_string(),
        git_date: git_date.to_string(),
        event: "applied".into(),
        error_text: String::new(),
    }
}

pub async fn ensure_history_index(conn: &mut TimingConn) -> Result<()> {
    conn.exec(sql::audit::BOOTSTRAP_INDEX).await
}

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
