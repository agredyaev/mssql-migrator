use std::sync::Mutex;

use super::{build_config, TomlConfig};

static ENV_LOCK: Mutex<()> = Mutex::new(());

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
