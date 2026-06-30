//! [`IoProfile`] — per-connection SQL I/O timing counters.

use serde::{Deserialize, Serialize};
use std::sync::{Mutex, MutexGuard};

/// Driver-level SQL I/O counters (timing connection boundary stats).
#[derive(Debug, Default, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
pub struct IoProfile {
    /// Cumulative time spent in SQL query calls, in milliseconds.
    pub query_ms: i64,
    /// Number of SQL query calls made.
    pub query_calls: i64,
    /// Cumulative time spent in SQL exec calls, in milliseconds.
    pub exec_ms: i64,
    /// Number of SQL exec calls made.
    pub exec_calls: i64,
    /// Number of extra connection attempts beyond the primary connection.
    pub extra_connects: i64,
    /// Cumulative time spent establishing extra connections, in milliseconds.
    pub extra_connect_ms: i64,
}

impl IoProfile {
    /// Returns total time spent at the SQL boundary (query + exec), in milliseconds.
    pub fn db_boundary_ms(&self) -> i64 {
        self.query_ms + self.exec_ms
    }

    /// Accumulates counters from `other` into `self`.
    pub fn merge_from(&mut self, other: Self) {
        self.query_ms += other.query_ms;
        self.query_calls += other.query_calls;
        self.exec_ms += other.exec_ms;
        self.exec_calls += other.exec_calls;
        self.extra_connects += other.extra_connects;
        self.extra_connect_ms += other.extra_connect_ms;
    }
}

pub(crate) fn lock_profile(io: &Mutex<IoProfile>) -> MutexGuard<'_, IoProfile> {
    io.lock().unwrap_or_else(|poisoned| poisoned.into_inner())
}

#[cfg(test)]
mod tests {
    use super::{lock_profile, IoProfile};
    use std::sync::{Arc, Mutex};

    #[test]
    fn lock_profile_recovers_poisoned_mutex_regression() {
        let io = Arc::new(Mutex::new(IoProfile::default()));
        let poisoned = Arc::clone(&io);
        let _ = std::thread::spawn(move || {
            let _guard = poisoned.lock().expect("test lock");
            panic!("poison io profile");
        })
        .join();

        lock_profile(&io).query_calls = 7;
        assert_eq!(lock_profile(&io).query_calls, 7);
    }
}
