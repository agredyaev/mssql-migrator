//! Shared SQL Server integration helpers (e2e + SLO).

mod rmigd;

#[path = "integration_config.rs"]
mod integration_config;

#[path = "integration_enabled.rs"]
mod integration_enabled;

use std::sync::OnceLock;

use migrator_core::Config;

static PARITY_CFG: OnceLock<Config> = OnceLock::new();

pub fn integration_enabled() -> bool {
    integration_enabled::enabled()
}

pub fn repo_root() -> std::path::PathBuf {
    integration_config::repo_root()
}

pub fn config() -> &'static Config {
    PARITY_CFG.get_or_init(|| {
        let mut cfg = integration_config::parity_config_base();
        if let Some(sock) = rmigd::ensure_started() {
            cfg.session_socket = sock;
        }
        cfg
    })
}
