use serde::{Deserialize, Serialize};

/// Driver-level SQL I/O counters (mirrors Go `timingConn` boundary stats).
#[derive(Debug, Default, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
pub struct IoProfile {
    pub query_ms: i64,
    pub query_calls: i64,
    pub exec_ms: i64,
    pub exec_calls: i64,
    pub extra_connects: i64,
    pub extra_connect_ms: i64,
}

impl IoProfile {
    pub fn db_boundary_ms(&self) -> i64 {
        self.query_ms + self.exec_ms
    }
}
