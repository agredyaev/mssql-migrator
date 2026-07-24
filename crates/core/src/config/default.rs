use super::Config;

impl Default for Config {
    fn default() -> Self {
        Self {
            sql_root: String::new(),
            sql_base: String::new(),
            report_dir: String::new(),
            log_level: "info".into(),
            database: String::new(),
            server: String::new(),
            port: "1433".into(),
            user: String::new(),
            password: String::new(),
            session_socket: String::new(),
            session_token: String::new(),
            command_timeout: std::time::Duration::from_secs(30),
            lock_timeout: std::time::Duration::from_secs(60),
            encrypt: true,
            trust_server_certificate: false,
            report_sync: false,
            skip_git: false,
            inspect_full: false,
            catalog_cache: true,
            allow_adopt: false,
        }
    }
}
