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
pub struct PlanDbTrace {
    pub path: Option<PlanDbPath>,
    pub cache_load_ms: i64,
    pub checksums_batch_ms: i64,
    pub catalog_ms: i64,
    pub query_calls: i64,
    pub query_ms: i64,
    pub bootstrap: bool,
    pub scoped_hit: bool,
    pub catalog_queried: bool,
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

/// Append one trace record when `RMIG_PLAN_DB_TRACE=1`.
pub fn maybe_append_trace(label: &str, trace: &PlanDbTrace, parallel_wall_ms: i64) {
    if !trace_enabled() {
        return;
    }
    let root = std::env::var("RMIG_REPO_ROOT").unwrap_or_else(|_| {
        std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../..")
            .to_string_lossy()
            .into_owned()
    });
    let dir = std::path::PathBuf::from(root).join("ops/perf/artifacts");
    let _ = std::fs::create_dir_all(&dir);
    let path = dir.join("rust_plan_db_trace.json");
    let mut entries: Vec<serde_json::Value> = if path.is_file() {
        std::fs::read_to_string(&path)
            .ok()
            .and_then(|s| serde_json::from_str(&s).ok())
            .unwrap_or_default()
    } else {
        Vec::new()
    };
    entries.push(serde_json::json!({
        "label": label,
        "parallel_wall_ms": parallel_wall_ms,
        "path": trace.path_label(),
        "cache_load_ms": trace.cache_load_ms,
        "checksums_batch_ms": trace.checksums_batch_ms,
        "catalog_ms": trace.catalog_ms,
        "query_calls": trace.query_calls,
        "query_ms": trace.query_ms,
        "bootstrap": trace.bootstrap,
        "scoped_hit": trace.scoped_hit,
        "catalog_queried": trace.catalog_queried,
    }));
    if let Ok(data) = serde_json::to_string_pretty(&entries) {
        let _ = std::fs::write(path, data);
    }
}
