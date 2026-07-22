use std::sync::Mutex;

use migrator_core::config::{build_config, load_toml_config_required, TomlConfig};
use migrator_core::session::resolve_session_token;

static TEST_LOCK: Mutex<()> = Mutex::new(());

struct EnvGuard {
    _lock: std::sync::MutexGuard<'static, ()>,
    saved: Option<String>,
}

impl EnvGuard {
    fn new() -> Self {
        let lock = TEST_LOCK.lock().expect("session token test lock");
        let saved = std::env::var("RMIG_SESSION_TOKEN").ok();
        std::env::remove_var("RMIG_SESSION_TOKEN");
        Self { _lock: lock, saved }
    }
}

impl Drop for EnvGuard {
    fn drop(&mut self) {
        match self.saved.take() {
            Some(value) => std::env::set_var("RMIG_SESSION_TOKEN", value),
            None => std::env::remove_var("RMIG_SESSION_TOKEN"),
        }
    }
}

#[test]
fn process_environment_token_is_loaded_happy_path() {
    let _guard = EnvGuard::new();
    std::env::set_var("RMIG_SESSION_TOKEN", "shell-token");
    let cfg = build_config(&TomlConfig::default());
    assert_eq!(resolve_session_token(Some(&cfg)), "shell-token");
}

#[test]
fn missing_process_environment_token_stays_empty_negative_path() {
    let _guard = EnvGuard::new();
    let cfg = build_config(&TomlConfig::default());
    assert!(resolve_session_token(Some(&cfg)).is_empty());
}

#[test]
fn toml_session_token_is_rejected_with_env_guidance_regression() {
    let _guard = EnvGuard::new();
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("config.toml");
    std::fs::write(&path, "[session]\ntoken = 'must-not-be-read'\n").expect("write config");
    let err = load_toml_config_required(&path).expect_err("TOML token must be rejected");
    let message = err.to_string();
    assert!(message.contains("RMIG_SESSION_TOKEN"), "{message}");
    assert!(
        !message.contains("must-not-be-read"),
        "secret leaked: {message}"
    );
}
