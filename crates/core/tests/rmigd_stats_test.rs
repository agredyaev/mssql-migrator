//! The daemon's `Stats` metrics/health endpoint returns live counters over the
//! socket without touching SQL Server.
//!
//! Run:
//!   RMIG_RUN_SQLSERVER_INTEGRATION=1 RMIG_USE_RMIGD=1 cargo test -p migrator-core \
//!     --test rmigd_stats_test -- --nocapture --test-threads=1

fn repo_root() -> std::path::PathBuf {
    std::path::Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("../..")
        .canonicalize()
        .expect("repo root")
}

#[path = "common/rmigd.rs"]
mod rmigd;

use std::io::{BufRead, BufReader, Write};
use std::os::unix::net::UnixStream;

fn integration_enabled() -> bool {
    std::env::var("RMIG_RUN_SQLSERVER_INTEGRATION")
        .map(|v| v == "1" || v.eq_ignore_ascii_case("true"))
        .unwrap_or(false)
}

#[test]
fn stats_endpoint_reports_metrics_without_touching_sql() {
    if !integration_enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }
    let Some(socket) = rmigd::ensure_started() else {
        eprintln!("skip: RMIG_USE_RMIGD not set");
        return;
    };

    let stream = UnixStream::connect(&socket).expect("connect rmigd socket");
    let mut writer = stream.try_clone().expect("clone");
    let mut reader = BufReader::new(stream);
    let mut call = |req: &str| {
        writer.write_all(req.as_bytes()).expect("write");
        writer.write_all(b"\n").expect("newline");
        let mut line = String::new();
        reader.read_line(&mut line).expect("read");
        line
    };

    let auth = call(r#"{"op":"auth","token":"rmig-integration-test-token"}"#);
    assert!(auth.contains("\"ok\":true"), "auth: {auth}");
    let _ = call(r#"{"op":"ping"}"#);

    let resp = call(r#"{"op":"stats"}"#);
    let v: serde_json::Value = serde_json::from_str(&resp).expect("stats response json");
    assert_eq!(v["ok"], serde_json::json!(true), "{resp}");
    let stats: serde_json::Value =
        serde_json::from_str(v["stats"].as_str().expect("stats blob")).expect("metrics json");
    assert!(
        stats["requests"].as_u64().expect("requests") >= 1,
        "at least the ping was counted: {stats}"
    );
    assert!(stats["uptime_s"].is_u64(), "{stats}");
    assert_eq!(stats["warm_connection"], serde_json::json!(true), "{stats}");
}
