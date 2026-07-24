//! SQL Server integration: live plan SLO with shared DB warm-up.
//! Run: RMIG_RUN_SQLSERVER_INTEGRATION=1 cargo test -p migrator-core --test integration_plan -- --nocapture --test-threads=1

mod common;

#[path = "common/warm.rs"]
mod warm;

use migrator_core::config::{
    build_config, discover_catalog_databases, ensure_catalog_databases_exist, load_toml_config,
    validate_config,
};
use migrator_core::engine::{run_command, Command};
use migrator_core_dev::pprof::PprofGuard;
use tokio::sync::OnceCell;

static DB_ENSURE: OnceCell<()> = OnceCell::const_new();

/// Same env contract as `ops/perf/cli_phase.sh` / `make slo`.
fn ensure_slo_harness_env() {
    if std::env::var("RMIG_USE_RMIGD").is_err() {
        std::env::set_var("RMIG_USE_RMIGD", "1");
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
            let file =
                load_toml_config(&common::repo_root().join("config.toml")).expect("load config");
            let mut cfg = build_config(&file);
            validate_config(&mut cfg).expect("valid slo config");
            let dbs = discover_catalog_databases(&cfg.sql_root).expect("discover catalog dbs");
            ensure_catalog_databases_exist(&cfg, &dbs)
                .await
                .expect("ensure catalog databases");
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
    warm::warm_db_once().await;
    let slo_ms = std::env::var("RMIG_SLO_MAX_CLI_WALL_MS")
        .ok()
        .and_then(|value| value.parse().ok())
        .unwrap_or(150);
    let _prof = PprofGuard::new("plan_live_slo");
    let out = match run_command(Command::Plan, cfg).await {
        Ok(out) => out,
        Err(err) => panic!("live plan: {err}"),
    };
    eprintln!(
        "live_plan timings: {}",
        serde_json::to_string(&out.timings).unwrap()
    );
    assert!(
        out.timings.cli_wall_ms < slo_ms,
        "cli_wall {}ms >= SLO {}ms",
        out.timings.cli_wall_ms,
        slo_ms
    );

    // The second plan must still inspect live module state and meet the CLI SLO.
    let out = match run_command(Command::Plan, cfg).await {
        Ok(out) => out,
        Err(err) => panic!("second live plan: {err}"),
    };
    eprintln!(
        "live_plan timings: {}",
        serde_json::to_string(&out.timings).unwrap()
    );
    assert!(
        out.timings.plan_db_query_calls > 0,
        "module drift check must query SQL Server"
    );
    assert!(
        out.timings.cli_wall_ms < slo_ms,
        "live plan cli_wall {}ms",
        out.timings.cli_wall_ms
    );
}
