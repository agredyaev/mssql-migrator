//! DB warm-up helpers (used by `integration_plan` only).

use migrator_core::config::Config;
use migrator_core::engine::{run_command, Command};
use tokio::sync::OnceCell;

use crate::common::{config, integration_enabled};

static DB_WARM: OnceCell<()> = OnceCell::const_new();

pub fn l1_fingerprint(cfg: &Config) -> String {
    format!("{}_{}", cfg.server, cfg.database)
}

/// One `plan` run per test process: audit ensure + catalog + L1 populate (no DROP DATABASE).
pub async fn warm_db_once() {
    if !integration_enabled() {
        return;
    }
    DB_WARM
        .get_or_init(|| async {
            let cfg = config();
            let l1 = migrator_core::cache::l1::L1Cache::new(&cfg.l1_cache_dir);
            let fingerprint = l1_fingerprint(cfg);
            let _ = l1.invalidate_all(&fingerprint);
            // #region agent log
            super::debug_log(
                "H6",
                "crates/core/tests/common/warm.rs:warm_db_once",
                "warm db invalidated l1 before bootstrap run",
                serde_json::json!({
                    "fingerprint": fingerprint,
                    "l1_cache_dir": cfg.l1_cache_dir,
                    "database": cfg.database,
                }),
            );
            // #endregion
            let mut warm = cfg.clone();
            warm.set_inspect_full(false);
            let out = run_command(Command::Plan, &warm)
                .await
                .expect("integration warm_db_once plan");
            // #region agent log
            super::debug_log(
                "H6",
                "crates/core/tests/common/warm.rs:warm_db_once",
                "warm db plan timings captured",
                serde_json::json!({
                    "timings": serde_json::to_value(&out.timings).unwrap_or(serde_json::Value::Null),
                    "l1_hit": out.timings.l1_cache_hit(),
                }),
            );
            // #endregion
        })
        .await;
}
