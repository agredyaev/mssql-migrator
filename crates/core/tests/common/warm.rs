//! DB warm-up helpers (used by `integration_plan` only).

use migrator_core::engine::{run_command, Command};
use tokio::sync::OnceCell;

use crate::common::{config, integration_enabled};

static DB_WARM: OnceCell<()> = OnceCell::const_new();

/// One `plan` run per test process to warm SQL Server state without dropping the database.
pub async fn warm_db_once() {
    if !integration_enabled() {
        return;
    }
    DB_WARM
        .get_or_init(|| async {
            let cfg = config();
            let mut warm = cfg.clone();
            warm.inspect_full = false;
            run_command(Command::Plan, &warm)
                .await
                .expect("integration warm_db_once plan");
        })
        .await;
}
