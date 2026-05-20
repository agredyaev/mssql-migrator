mod catalog;
mod env;
mod validate;

use std::fmt;
use std::time::Duration;

pub use catalog::{
    discover_catalog_databases, ensure_catalog_databases_exist, normalize_catalog_paths,
    resolve_single_database,
};
pub use env::{build_config, load_env_file};
pub use validate::validate_config;

#[derive(Clone)]
pub struct Config {
    pub sql_root: String,
    pub sql_base: String,
    pub report_dir: String,
    pub report_sync: bool,
    pub log_level: String,
    pub server: String,
    pub port: String,
    pub database: String,
    pub db_auth: String,
    pub user: String,
    pub password: String,
    pub encrypt: bool,
    pub trust_server_certificate: bool,
    pub command_timeout: Duration,
    pub lock_timeout: Duration,
    pub skip_git: bool,
    pub json_logs: bool,
    pub inspect_full: bool,
    pub catalog_cache: bool,
    pub session_socket: String,
    pub session_token: String,
    pub l1_cache_dir: String,
    pub slo_max_cli_wall_ms: i64,
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
            server: String::new(),
            port: "1433".into(),
            database: String::new(),
            db_auth: "sql".into(),
            user: String::new(),
            password: String::new(),
            encrypt: false,
            trust_server_certificate: true,
            command_timeout: Duration::from_secs(30),
            lock_timeout: Duration::from_secs(60),
            skip_git: false,
            json_logs: false,
            inspect_full: false,
            catalog_cache: true,
            session_socket: String::new(),
            session_token: String::new(),
            l1_cache_dir: ".rmig/cache".into(),
            slo_max_cli_wall_ms: 100,
        }
    }
}
