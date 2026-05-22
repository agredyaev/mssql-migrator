use super::{trace_enabled, PlanDbTrace};

/// Append one trace record when `RMIG_PLAN_DB_TRACE=1`.
pub fn maybe_append_trace(label: &str, trace: &PlanDbTrace, parallel_wall_ms: i64) {
    if !trace_enabled() {
        return;
    }
    let root = std::env::var("RMIG_REPO_ROOT").unwrap_or_else(|_| {
        std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../..")
            .to_string_lossy()
            .into_owned()
    });
    let dir = std::path::PathBuf::from(root).join("ops/perf/artifacts");
    let _ = std::fs::create_dir_all(&dir);
    let path = dir.join("plan_db_trace.json");
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
        "cache_load_ms": trace.timings.cache_load_ms,
        "checksums_batch_ms": trace.timings.checksums_batch_ms,
        "catalog_ms": trace.timings.catalog_ms,
        "catalog_sql_ms": trace.timings.catalog_sql_ms,
        "intern_catalog_ms": trace.timings.intern_catalog_ms,
        "query_calls": trace.timings.query_calls,
        "query_ms": trace.timings.query_ms,
        "round_trips": trace.timings.round_trips,
        "bootstrap": trace.flags.bootstrap,
        "scoped_hit": trace.flags.scoped_hit,
        "catalog_queried": trace.flags.catalog_queried,
        "history_empty": trace.flags.history_empty,
        "checksums_skipped": trace.flags.checksums_skipped,
    }));
    if let Ok(data) = serde_json::to_string_pretty(&entries) {
        let _ = std::fs::write(path, data);
    }
}
