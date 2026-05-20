use serde::{Deserialize, Serialize};

/// Wall durations for plan/CLI profiling (milliseconds). Mirrors Go `prodgate.PhaseTimings`.
#[derive(Debug, Default, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
pub struct PhaseTimings {
    #[serde(skip_serializing_if = "is_zero")]
    pub connect_ms: i64,
    #[serde(skip_serializing_if = "is_zero")]
    pub scan_ms: i64,
    #[serde(skip_serializing_if = "is_zero")]
    pub inspect_ms: i64,
    #[serde(skip_serializing_if = "is_zero")]
    pub checksums_ms: i64,
    #[serde(skip_serializing_if = "is_zero")]
    pub ensure_ms: i64,
    #[serde(skip_serializing_if = "is_zero")]
    pub parallel_wall_ms: i64,
    #[serde(skip_serializing_if = "is_zero")]
    pub audit_ms: i64,
    #[serde(skip_serializing_if = "is_zero")]
    pub diff_ms: i64,
    #[serde(skip_serializing_if = "is_zero")]
    pub plan_wall_ms: i64,
    #[serde(skip_serializing_if = "is_zero")]
    pub cli_wall_ms: i64,
    #[serde(skip_serializing_if = "is_zero")]
    pub engine_ms: i64,
    #[serde(skip_serializing_if = "is_zero")]
    pub apply_ms: i64,
    #[serde(skip_serializing_if = "is_zero")]
    pub audit_flush_ms: i64,
    #[serde(skip_serializing_if = "is_false")]
    pub l1_cache_hit: bool,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub plan_db_path: String,
    #[serde(skip_serializing_if = "is_zero")]
    pub plan_db_query_calls: i64,
    #[serde(skip_serializing_if = "is_zero")]
    pub plan_db_query_ms: i64,
    #[serde(skip_serializing_if = "is_false")]
    pub plan_db_bootstrap: bool,
    #[serde(skip_serializing_if = "is_false")]
    pub plan_db_catalog_queried: bool,
    #[serde(skip_serializing_if = "is_zero")]
    pub plan_db_checksums_batch_ms: i64,
    #[serde(skip_serializing_if = "is_zero")]
    pub plan_db_catalog_ms: i64,
}

fn is_zero(v: &i64) -> bool {
    *v == 0
}

fn is_false(v: &bool) -> bool {
    !*v
}

impl PhaseTimings {
    pub fn finish_audit_ms(&mut self) {
        self.audit_ms = self.ensure_ms + self.checksums_ms;
    }
}

pub fn dur_ms(d: std::time::Duration) -> i64 {
    d.as_millis() as i64
}
