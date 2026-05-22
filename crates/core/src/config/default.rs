use std::sync::Arc;

use super::cold::ConfigCold;
use super::flags::{CONFIG_COLD_FLAG_TRUST_SERVER_CERTIFICATE, CONFIG_FLAG_CATALOG_CACHE};
use super::Config;

impl Default for Config {
    fn default() -> Self {
        Self {
            sql_root: String::new(),
            sql_base: String::new(),
            report_dir: String::new(),
            log_level: "info".into(),
            database: String::new(),
            slo_max_cli_wall_ms: 150,
            flags: CONFIG_FLAG_CATALOG_CACHE,
            cold: Arc::new(ConfigCold {
                port: "1433".into(),
                db_auth: "sql".into(),
                cold_flags: CONFIG_COLD_FLAG_TRUST_SERVER_CERTIFICATE,
                command_timeout: std::time::Duration::from_secs(30),
                lock_timeout: std::time::Duration::from_secs(60),
                l1_cache_dir: ".rmig/cache".into(),
                ..ConfigCold::default()
            }),
        }
    }
}
