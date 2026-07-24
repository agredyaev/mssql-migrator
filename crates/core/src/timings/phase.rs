//! [`PhaseTimings`] — flat struct of per-phase wall durations in milliseconds.
//!
//! ### Purpose
//! Accumulates wall-clock measurements for every `rmig` execution phase so
//! SLO audits and JSON reporting can inspect where time is spent. Zero-valued
//! durations and `false` flags are omitted from the JSON for compact output.

use serde::{Deserialize, Serialize};

/// Wall durations for plan/CLI profiling (milliseconds).
#[derive(Debug, Default, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(default)]
pub struct PhaseTimings {
    /// Time to establish a TDS connection.
    #[serde(skip_serializing_if = "is_zero")]
    pub connect_ms: i64,
    /// Time to scan the SQL tree on disk.
    #[serde(skip_serializing_if = "is_zero")]
    pub scan_ms: i64,
    /// Time to inspect the database catalog.
    #[serde(skip_serializing_if = "is_zero")]
    pub inspect_ms: i64,
    /// Time to load or compute checksums.
    #[serde(skip_serializing_if = "is_zero")]
    pub checksums_ms: i64,
    /// Time to ensure plan-DB schema and tables.
    #[serde(skip_serializing_if = "is_zero")]
    pub ensure_ms: i64,
    /// Total parallel wall time across plan-DB queries.
    #[serde(skip_serializing_if = "is_zero")]
    pub parallel_wall_ms: i64,
    /// Time for audit-history operations.
    #[serde(skip_serializing_if = "is_zero")]
    pub audit_ms: i64,
    /// Time to compute the diff (plan vs DB state).
    #[serde(skip_serializing_if = "is_zero")]
    pub diff_ms: i64,
    /// Total wall time for the plan phase end-to-end.
    #[serde(skip_serializing_if = "is_zero")]
    pub plan_wall_ms: i64,
    /// Wall time for the CLI driver (full entry-point).
    #[serde(skip_serializing_if = "is_zero")]
    pub cli_wall_ms: i64,
    /// Time spent in the engine run loop.
    #[serde(skip_serializing_if = "is_zero")]
    pub engine_ms: i64,
    /// Time to apply changes to the database.
    #[serde(skip_serializing_if = "is_zero")]
    pub apply_ms: i64,
    /// Number of plan-DB query calls made.
    #[serde(skip_serializing_if = "is_zero")]
    pub plan_db_query_calls: i64,
    /// Cumulative time for plan-DB query round-trips.
    #[serde(skip_serializing_if = "is_zero")]
    pub plan_db_query_ms: i64,
    /// Time for plan-DB checksums batch load.
    #[serde(skip_serializing_if = "is_zero")]
    pub plan_db_checksums_batch_ms: i64,
    /// Time for plan-DB catalog queries.
    #[serde(skip_serializing_if = "is_zero")]
    pub plan_db_catalog_ms: i64,
    /// Time for plan-DB catalog SQL execution.
    #[serde(skip_serializing_if = "is_zero")]
    pub plan_db_catalog_sql_ms: i64,
    /// Number of TDS round-trips made during plan-DB.
    #[serde(skip_serializing_if = "is_zero")]
    pub plan_db_round_trips: i64,
    /// Path to the plan-DB trace file.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub plan_db_path: String,
    /// True when the plan-DB was bootstrapped (tables created) in this run.
    #[serde(skip_serializing_if = "std::ops::Not::not")]
    pub plan_db_bootstrap: bool,
    /// True when the plan-DB catalog was queried (not fully cached).
    #[serde(skip_serializing_if = "std::ops::Not::not")]
    pub plan_db_catalog_queried: bool,
    /// True when the audit-history table was empty at plan time.
    #[serde(skip_serializing_if = "std::ops::Not::not")]
    pub plan_db_history_empty: bool,
    /// True when checksum loading was skipped (no audit tables).
    #[serde(skip_serializing_if = "std::ops::Not::not")]
    pub plan_db_checksums_skipped: bool,
}

fn is_zero(v: &i64) -> bool {
    *v == 0
}

impl PhaseTimings {
    /// Sets `audit_ms` to the sum of `ensure_ms + checksums_ms`.
    pub fn finish_audit_ms(&mut self) {
        self.audit_ms = self.ensure_ms + self.checksums_ms;
    }
}
