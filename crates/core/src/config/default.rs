use std::sync::Arc;

use super::cold::ConfigCold;
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
            report_sync: false,
            skip_git: false,
            json_logs: false,
            inspect_full: false,
            catalog_cache: true,
            allow_adopt: false,
            cold: Arc::new(ConfigCold {
                port: "1433".into(),
                db_auth: "sql".into(),
                // Encrypt by default and require normal certificate validation.
                // Local development may opt out explicitly.
                encrypt: true,
                command_timeout: std::time::Duration::from_secs(30),
                lock_timeout: std::time::Duration::from_secs(60),
                l1_cache_dir: ".rmig/cache".into(),
                ..ConfigCold::default()
            }),
        }
    }
}
