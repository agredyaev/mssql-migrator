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
}

/// Locks `m`, recovering the guard if a prior panic poisoned the mutex.
pub(crate) fn lock_unpoisoned<T>(m: &Mutex<T>) -> MutexGuard<'_, T> {
    m.lock().unwrap_or_else(|poisoned| poisoned.into_inner())
}

#[cfg(test)]
mod tests {
    use super::{lock_unpoisoned, IoProfile};
    use std::sync::{Arc, Mutex};

    #[test]
    fn lock_unpoisoned_recovers_poisoned_mutex_regression() {
        let io = Arc::new(Mutex::new(IoProfile::default()));
        let poisoned = Arc::clone(&io);
        let _ = std::thread::spawn(move || {
            let _guard = poisoned.lock().expect("test lock");
            panic!("poison io profile");
        })
        .join();

        lock_unpoisoned(&io).query_calls = 7;
        assert_eq!(lock_unpoisoned(&io).query_calls, 7);
    }
}
