use std::sync::Mutex;

use super::{build_config, TomlConfig};

static ENV_LOCK: Mutex<()> = Mutex::new(());

#[test]
fn connection_security_defaults_fail_closed() {
    let cfg = crate::Config::default();
    assert!(cfg.encrypt, "TLS encryption must default on");
    assert!(
        !cfg.trust_server_certificate,
        "certificate validation must default on"
    );
}

#[test]
fn checked_in_config_contains_only_non_peer_settings() {
    let file: TomlConfig = toml::from_str(include_str!("../../../../config.toml"))
        .expect("checked-in config must parse");
    assert_eq!(file.paths.sql_root.as_deref(), Some(".temp/sql"));
    assert_eq!(file.execution.log_level.as_deref(), Some("info"));
}

#[test]
fn peer_settings_come_from_process_environment_regression() {
    let _lock = ENV_LOCK.lock().expect("env lock");
    let saved = std::env::var("RM_DB_SERVER").ok();
    std::env::set_var("RM_DB_SERVER", "env.example");
    let file: TomlConfig = toml::from_str("[paths]\nsql_root = 'sql'\n").expect("parse config");
    let cfg = build_config(&file);
    match saved {
        Some(value) => std::env::set_var("RM_DB_SERVER", value),
        None => std::env::remove_var("RM_DB_SERVER"),
    }
    assert_eq!(cfg.server, "env.example");
    assert_eq!(cfg.sql_root, "sql");
}

#[test]
fn adoption_opt_in_comes_from_process_environment_regression() {
    let _lock = ENV_LOCK.lock().expect("env lock");
    let saved = std::env::var("RMIG_ALLOW_ADOPT").ok();
    std::env::set_var("RMIG_ALLOW_ADOPT", "true");
    let cfg = build_config(&TomlConfig::default());
    match saved {
        Some(value) => std::env::set_var("RMIG_ALLOW_ADOPT", value),
        None => std::env::remove_var("RMIG_ALLOW_ADOPT"),
    }
    assert!(cfg.allow_adopt);
}
