//! SQL Server integration: plan SLO and L1 hit in one suite (shared DB warm-up).
//! Run: RMIG_RUN_SQLSERVER_INTEGRATION=1 cargo test -p migrator-core --test integration_plan -- --nocapture --test-threads=1

mod common;

#[path = "common/warm.rs"]
mod warm;

use std::fs::{create_dir_all, OpenOptions};
use std::io::Write;
use std::path::PathBuf;
use std::time::{SystemTime, UNIX_EPOCH};

use migrator_core::config::{
    build_config, discover_catalog_databases, ensure_catalog_databases_exist, load_env_file,
    validate_config,
};
use migrator_core::engine::{run_command, Command};
use migrator_core_dev::pprof::PprofGuard;
use tokio::sync::OnceCell;

static DB_ENSURE: OnceCell<()> = OnceCell::const_new();

fn debug_log(hypothesis_id: &str, location: &str, message: &str, data: serde_json::Value) {
    let payload = serde_json::json!({
        "sessionId": "1200a9",
        "runId": std::env::var("RMIG_DEBUG_RUN_ID").unwrap_or_else(|_| "manual".into()),
        "hypothesisId": hypothesis_id,
        "location": location,
        "message": message,
        "data": data,
        "timestamp": SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map(|duration| duration.as_millis())
            .unwrap_or_default(),
    });
    let debug_log = std::env::var("RMIG_DEBUG_LOG").unwrap_or_else(|_| {
        let cwd = std::env::current_dir()
            .unwrap_or_else(|_| PathBuf::from("."));
        format!("{}/.cursor/debug-1200a9.log", cwd.display())
    });
    if let Some(parent) = PathBuf::from(&debug_log).parent() {
        let _ = create_dir_all(parent);
    }
    if let Ok(mut file) = OpenOptions::new()
        .create(true)
        .append(true)
        .open(&debug_log)
    {
        let mut line = payload.to_string();
        line.push('\n');
        let _ = file.write_all(line.as_bytes());
    }
}

/// Same env contract as `ops/perf/cli_phase.sh` / `make slo`.
fn ensure_slo_harness_env() {
    if std::env::var("RMIG_USE_RMIGD").is_err() {
        std::env::set_var("RMIG_USE_RMIGD", "1");
    }
    if std::env::var("RMIG_INTEGRATION_WARM_SNAPSHOT").is_err() {
        std::env::set_var("RMIG_INTEGRATION_WARM_SNAPSHOT", "1");
    }
    if std::env::var("RM_SQL_ROOT").is_err() {
        let sql = common::repo_root().join(".temp/sql");
        let s = sql.to_string_lossy().into_owned();
        std::env::set_var("RM_SQL_ROOT", &s);
        std::env::set_var("RM_SQL_BASE", &s);
    }
    if std::env::var("RM_DB_SERVER").is_err() {
        std::env::set_var("RM_DB_SERVER", "127.0.0.1");
    }
    if std::env::var("RM_DB_PORT").is_err() {
        std::env::set_var("RM_DB_PORT", "1433");
    }
    if std::env::var("RM_DB_USER").is_err() {
        std::env::set_var("RM_DB_USER", "sa");
    }
    if std::env::var("RM_DB_PASSWORD").is_err() {
        std::env::set_var("RM_DB_PASSWORD", "yourStrong(!)Password");
    }
}

async fn ensure_catalog_databases_ready() {
    DB_ENSURE
        .get_or_init(|| async {
            let env = load_env_file(&common::repo_root().join(".env")).unwrap_or_default();
            let mut cfg = build_config(&env, true);
            validate_config(&mut cfg).expect("valid slo config");
            let dbs = discover_catalog_databases(&cfg.sql_root).expect("discover catalog dbs");
            // #region agent log
            debug_log(
                "H7",
                "crates/core/tests/integration_plan.rs:ensure_catalog_databases_ready",
                "catalog databases discovered for slo suite",
                serde_json::json!({
                    "sql_root": cfg.sql_root,
                    "database_count": dbs.len(),
                    "databases": dbs,
                }),
            );
            // #endregion
            ensure_catalog_databases_exist(&cfg, &dbs)
                .await
                .expect("ensure catalog databases");
            // #region agent log
            debug_log(
                "H7",
                "crates/core/tests/integration_plan.rs:ensure_catalog_databases_ready",
                "catalog databases ensured for slo suite",
                serde_json::json!({
                    "sql_root": cfg.sql_root,
                }),
            );
            // #endregion
        })
        .await;
}

#[tokio::test]
async fn integration_plan_sqlserver_suite() {
    if !common::integration_enabled() {
        eprintln!("skip: RMIG_RUN_SQLSERVER_INTEGRATION not set");
        return;
    }
    ensure_slo_harness_env();
    ensure_catalog_databases_ready().await;
    let cfg = common::config();
    // #region agent log
    debug_log(
        "H5",
        "crates/core/tests/integration_plan.rs:integration_plan_sqlserver_suite",
        "slo suite configured",
        serde_json::json!({
            "slo_ms": cfg.slo_max_cli_wall_ms,
            "session_socket_present": !cfg.session_socket.is_empty(),
            "session_socket": cfg.session_socket,
            "database": cfg.database,
            "catalog_cache": cfg.catalog_cache(),
            "inspect_full": cfg.inspect_full(),
        }),
    );
    // #endregion
    warm::warm_db_once().await;

    let l1 = migrator_core::cache::l1::L1Cache::new(&cfg.l1_cache_dir);
    let fp = warm::l1_fingerprint(cfg);

    // Cache-miss SLO: invalidate L1 only; SQL catalog stays warm from warm_db_once.
    let _ = l1.invalidate_all(&fp);
    // #region agent log
    debug_log(
        "H6",
        "crates/core/tests/integration_plan.rs:integration_plan_sqlserver_suite",
        "l1 invalidated before cache miss run",
        serde_json::json!({
            "fingerprint": fp,
            "l1_cache_dir": cfg.l1_cache_dir,
        }),
    );
    // #endregion
    let _prof = PprofGuard::new("plan_cache_miss_slo");
    let out = match run_command(Command::Plan, cfg).await {
        Ok(out) => out,
        Err(err) => {
            // #region agent log
            debug_log(
                "H8",
                "crates/core/tests/integration_plan.rs:integration_plan_sqlserver_suite",
                "cache miss plan command failed",
                serde_json::json!({
                    "error": err.to_string(),
                }),
            );
            // #endregion
            panic!("plan cache miss: {err}");
        }
    };
    // #region agent log
    debug_log(
        "H8",
        "crates/core/tests/integration_plan.rs:integration_plan_sqlserver_suite",
        "cache miss timings captured",
        serde_json::json!({
            "slo_ms": cfg.slo_max_cli_wall_ms,
            "slo_exceeded": out.timings.cli_wall_ms >= cfg.slo_max_cli_wall_ms,
            "timings": serde_json::to_value(&out.timings).unwrap_or(serde_json::Value::Null),
        }),
    );
    // #endregion
    eprintln!(
        "cache_miss timings: {}",
        serde_json::to_string(&out.timings).unwrap()
    );
    assert!(
        out.timings.cli_wall_ms < cfg.slo_max_cli_wall_ms,
        "cli_wall {}ms >= SLO {}ms",
        out.timings.cli_wall_ms,
        cfg.slo_max_cli_wall_ms
    );
    assert!(!out.timings.l1_cache_hit());

    // L1 hit: second plan should be fast without reconnecting to an empty DB.
    let out = match run_command(Command::Plan, cfg).await {
        Ok(out) => out,
        Err(err) => {
            // #region agent log
            debug_log(
                "H8",
                "crates/core/tests/integration_plan.rs:integration_plan_sqlserver_suite",
                "l1 hit plan command failed",
                serde_json::json!({
                    "error": err.to_string(),
                }),
            );
            // #endregion
            panic!("plan l1 hit: {err}");
        }
    };
    // #region agent log
    debug_log(
        "H8",
        "crates/core/tests/integration_plan.rs:integration_plan_sqlserver_suite",
        "l1 hit timings captured",
        serde_json::json!({
            "slo_ms": cfg.slo_max_cli_wall_ms,
            "slo_exceeded": out.timings.cli_wall_ms >= cfg.slo_max_cli_wall_ms,
            "timings": serde_json::to_value(&out.timings).unwrap_or(serde_json::Value::Null),
        }),
    );
    // #endregion
    eprintln!(
        "l1_hit timings: {}",
        serde_json::to_string(&out.timings).unwrap()
    );
    assert!(out.timings.l1_cache_hit());
    assert!(
        out.timings.cli_wall_ms < cfg.slo_max_cli_wall_ms,
        "L1 hit cli_wall {}ms",
        out.timings.cli_wall_ms
    );
}
