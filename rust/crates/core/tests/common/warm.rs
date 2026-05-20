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
            let _ = l1.invalidate_all(&l1_fingerprint(cfg));
            let mut warm = cfg.clone();
            warm.inspect_full = true;
            run_command(Command::Plan, &warm)
                .await
                .expect("integration warm_db_once plan");
        })
        .await;
}
