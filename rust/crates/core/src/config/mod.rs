mod catalog;
mod cold;
mod env;
mod validate;

use std::fmt;
use std::ops::{Deref, DerefMut};
use std::sync::Arc;

pub use catalog::{
    discover_catalog_databases, ensure_catalog_databases_exist, normalize_catalog_paths,
    resolve_single_database,
};
pub use cold::ConfigCold;
pub use env::{build_config, load_env_file};
pub use validate::validate_config;

/// Hot run flags + layout paths (**C1**); connection block in [`ConfigCold`] behind `Arc`.
#[derive(Clone)]
pub struct Config {
    pub sql_root: String,
    pub sql_base: String,
    pub report_dir: String,
    pub report_sync: bool,
    pub log_level: String,
    pub database: String,
    pub skip_git: bool,
    pub json_logs: bool,
    pub inspect_full: bool,
    pub catalog_cache: bool,
    pub slo_max_cli_wall_ms: i64,
    pub(crate) cold: Arc<ConfigCold>,
}

impl Deref for Config {
    type Target = ConfigCold;
    fn deref(&self) -> &ConfigCold {
        &self.cold
    }
}

impl DerefMut for Config {
    fn deref_mut(&mut self) -> &mut ConfigCold {
        Arc::make_mut(&mut self.cold)
    }
}

impl fmt::Debug for Config {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("Config")
            .field("sql_root", &self.sql_root)
            .field("sql_base", &self.sql_base)
            .field("report_dir", &self.report_dir)
            .field("server", &self.server)
            .field("port", &self.port)
            .field("database", &self.database)
            .field("user", &self.user)
            .field("password", &"<redacted>")
            .field("session_socket", &self.session_socket)
            .field("session_token", &mask_token(&self.session_token))
            .field("encrypt", &self.encrypt)
            .field("trust_server_certificate", &self.trust_server_certificate)
            .finish_non_exhaustive()
    }
}

fn mask_token(token: &str) -> &'static str {
    if token.is_empty() {
        "<unset>"
    } else {
        "<redacted>"
    }
}

impl Default for Config {
    fn default() -> Self {
        Self {
            sql_root: String::new(),
            sql_base: String::new(),
            report_dir: String::new(),
            report_sync: false,
            log_level: "info".into(),
            database: String::new(),
            skip_git: false,
            json_logs: false,
            inspect_full: false,
            catalog_cache: true,
            slo_max_cli_wall_ms: 150,
            cold: Arc::new(ConfigCold {
                port: "1433".into(),
                db_auth: "sql".into(),
                trust_server_certificate: true,
                command_timeout: std::time::Duration::from_secs(30),
                lock_timeout: std::time::Duration::from_secs(60),
                l1_cache_dir: ".rmig/cache".into(),
                ..ConfigCold::default()
            }),
        }
    }
}
