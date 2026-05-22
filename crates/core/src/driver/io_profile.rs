use serde::{Deserialize, Serialize};

/// Driver-level SQL I/O counters (timing connection boundary stats).
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

    pub fn merge_from(&mut self, other: Self) {
        self.query_ms += other.query_ms;
        self.query_calls += other.query_calls;
        self.exec_ms += other.exec_ms;
        self.exec_calls += other.exec_calls;
        self.extra_connects += other.extra_connects;
        self.extra_connect_ms += other.extra_connect_ms;
    }
}
