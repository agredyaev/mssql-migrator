#![allow(missing_docs)]

mod append;

use serde::{Deserialize, Serialize};

/// Plan DB execution path (for perf regression analysis).
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum PlanDbPath {
    CacheHit,
    WarmSnapshot,
    GitDelta,
    ColdFull,
    Incremental,
}

impl PlanDbPath {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::CacheHit => "cache_hit",
            Self::WarmSnapshot => "warm_snapshot",
            Self::GitDelta => "git_delta",
            Self::ColdFull => "cold_full",
            Self::Incremental => "incremental",
        }
    }
}

#[derive(Debug, Default, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
pub struct PlanDbTimings {
    pub cache_load_ms: i64,
    pub checksums_batch_ms: i64,
    pub catalog_ms: i64,
    pub catalog_sql_ms: i64,
    pub intern_catalog_ms: i64,
    pub query_calls: i64,
    pub query_ms: i64,
    pub round_trips: i64,
}

#[derive(Debug, Default, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
pub struct PlanDbFlags {
    pub bootstrap: bool,
    pub scoped_hit: bool,
    pub catalog_queried: bool,
    pub history_empty: bool,
    pub checksums_skipped: bool,
}

#[derive(Debug, Default, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
pub struct PlanDbTrace {
    pub path: Option<PlanDbPath>,
    #[serde(flatten)]
    pub timings: PlanDbTimings,
    #[serde(flatten)]
    pub flags: PlanDbFlags,
}

impl PlanDbTrace {
    pub fn path_label(&self) -> &str {
        self.path.map(PlanDbPath::as_str).unwrap_or("unknown")
    }
}

pub fn trace_enabled() -> bool {
    matches!(
        std::env::var("RMIG_PLAN_DB_TRACE").as_deref(),
        Ok("1") | Ok("true") | Ok("yes")
    )
}

pub fn max_parallel_wall_ms() -> i64 {
    std::env::var("RMIG_PLAN_DB_MAX_PAR_MS")
        .ok()
        .and_then(|s| s.trim().parse().ok())
        .unwrap_or(500)
}

pub fn plan_db_path_from_label(label: &str) -> Option<PlanDbPath> {
    match label {
        "cache_hit" => Some(PlanDbPath::CacheHit),
        "warm_snapshot" => Some(PlanDbPath::WarmSnapshot),
        "git_delta" => Some(PlanDbPath::GitDelta),
        "cold_full" => Some(PlanDbPath::ColdFull),
        "incremental" => Some(PlanDbPath::Incremental),
        _ => None,
    }
}

/// Warm-path SLO applies to incremental, git_delta, and cache hits — not cold full inspect.
pub fn plan_db_slo_exempt(path_label: &str, l1_cache_hit: bool) -> bool {
    l1_cache_hit || path_label == "cold_full"
}

pub use append::maybe_append_trace;
