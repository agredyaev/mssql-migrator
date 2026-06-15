//! [`ApplyResult`] — outcome of a single apply phase.
//!
//! ### Purpose
//! Tracks the number of objects applied, skipped, and failed, along with
//! error messages and audit history records generated during the apply.

use crate::audit::HistoryRecord;

/// Aggregate apply outcome: row counts, errors, and audit history.
#[derive(Debug, Default)]
pub struct ApplyResult {
    /// Number of objects successfully applied.
    pub applied: usize,
    /// Number of objects skipped (unchanged).
    pub skipped: usize,
    /// Number of objects that failed to apply.
    pub failed: usize,
    /// Error messages from failed applies.
    pub errors: Vec<String>,
    /// Audit history records written during this apply.
    pub history: Vec<HistoryRecord>,
}

impl ApplyResult {
    /// Record a failure: increment counter and append the error message.
    pub fn push_error(&mut self, msg: String) {
        self.failed += 1;
        self.errors.push(msg);
    }
}
