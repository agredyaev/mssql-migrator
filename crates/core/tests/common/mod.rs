//! Shared SQL Server integration helpers (e2e + SLO).

#[allow(dead_code)]
mod rmigd;

#[path = "integration_config.rs"]
mod integration_config;

#[path = "integration_enabled.rs"]
mod integration_enabled;

use std::sync::OnceLock;

use migrator_core::Config;

#[allow(dead_code)]
static DIRECT_CFG: OnceLock<Config> = OnceLock::new();
#[allow(dead_code)]
static PARITY_CFG: OnceLock<Config> = OnceLock::new();

pub fn integration_enabled() -> bool {
    integration_enabled::enabled()
}

pub fn repo_root() -> std::path::PathBuf {
    integration_config::repo_root()
}

#[allow(dead_code)]
pub fn config() -> &'static Config {
    PARITY_CFG.get_or_init(|| {
        let mut cfg = integration_config::parity_config_base();
        if let Some(sock) = rmigd::ensure_started() {
            cfg.session_socket = sock;
        }
        cfg
    })
}

#[allow(dead_code)]
pub fn direct_config() -> &'static Config {
    DIRECT_CFG.get_or_init(integration_config::parity_config_base)
}
