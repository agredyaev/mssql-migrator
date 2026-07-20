use std::sync::Mutex;

use super::{build_config, TomlConfig};

static ENV_LOCK: Mutex<()> = Mutex::new(());

#[test]
fn connection_security_defaults_fail_closed() {
    let cfg = crate::Config::default();
    assert!(cfg.encrypt(), "TLS encryption must default on");
    assert!(
        !cfg.trust_server_certificate(),
        "certificate validation must default on"
    );
}

#[test]
fn checked_in_config_keeps_secure_transport_regression() {
    let file: TomlConfig = toml::from_str(include_str!("../../../../config.toml"))
        .expect("checked-in config must parse");
    assert_eq!(file.database.encrypt, Some(true));
    assert_eq!(file.database.trust_server_certificate, Some(false));
}

#[test]
fn process_environment_overrides_toml_regression() {
    let _lock = ENV_LOCK.lock().expect("env lock");
    let saved = std::env::var("RM_DB_SERVER").ok();
    std::env::set_var("RM_DB_SERVER", "env.example");
    let file: TomlConfig =
        toml::from_str("[database]\nserver = 'file.example'\n[paths]\nsql_root = 'sql'\n")
            .expect("parse config");
    let cfg = build_config(&file, false);
    match saved {
        Some(value) => std::env::set_var("RM_DB_SERVER", value),
        None => std::env::remove_var("RM_DB_SERVER"),
    }
    assert_eq!(cfg.server, "env.example");
    assert_eq!(cfg.sql_root, "sql");
}
