//! SQL Server integration: plan SLO and L1 hit in one suite (shared DB warm-up).
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
            let file =
                load_toml_config(&common::repo_root().join("config.toml")).expect("load config");
            let mut cfg = build_config(&file, true);
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

    let l1 = migrator_core::cache::l1::L1Cache::new(&cfg.l1_cache_dir);
    let fp = warm::l1_fingerprint(cfg);

    // Cache-miss SLO: invalidate L1 only; SQL catalog stays warm from warm_db_once.
    let _ = l1.invalidate_all(&fp);
    let _prof = PprofGuard::new("plan_cache_miss_slo");
    let out = match run_command(Command::Plan, cfg).await {
        Ok(out) => out,
        Err(err) => panic!("plan cache miss: {err}"),
    };
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

    // Module workspaces must bypass L1 so out-of-band definition drift remains
    // visible; the second live plan must still meet the CLI SLO.
    let out = match run_command(Command::Plan, cfg).await {
        Ok(out) => out,
        Err(err) => panic!("second live plan: {err}"),
    };
    eprintln!(
        "live_plan timings: {}",
        serde_json::to_string(&out.timings).unwrap()
    );
    assert!(!out.timings.l1_cache_hit());
    assert!(
        out.timings.plan_db_query_calls > 0,
        "module drift check must query SQL Server"
    );
    assert!(
        out.timings.cli_wall_ms < cfg.slo_max_cli_wall_ms,
        "live plan cli_wall {}ms",
        out.timings.cli_wall_ms
    );
}
