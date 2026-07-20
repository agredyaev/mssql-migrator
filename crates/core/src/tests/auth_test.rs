use super::*;
use crate::config::{Config, ConfigCold};
use std::sync::{Arc, Mutex};

static TEST_LOCK: Mutex<()> = Mutex::new(());

struct EnvGuard {
    _lock: std::sync::MutexGuard<'static, ()>,
}

impl EnvGuard {
    fn new() -> Self {
        let lock = TEST_LOCK.lock().expect("session auth test lock");
        std::env::remove_var("RMIG_SESSION_TOKEN");
        super::reset_session_token_for_test();
        Self { _lock: lock }
    }
}

impl Drop for EnvGuard {
    fn drop(&mut self) {
        std::env::remove_var("RMIG_SESSION_TOKEN");
        super::reset_session_token_for_test();
    }
}

fn cfg_with_token(token: &str) -> Config {
    Config {
        cold: Arc::new(ConfigCold {
            session_token: token.into(),
            ..ConfigCold::default()
        }),
        ..Config::default()
    }
}

#[test]
fn resolve_session_token_prefers_config_happy_path() {
    let _guard = EnvGuard::new();
    std::env::set_var("RMIG_SESSION_TOKEN", "from-env");
    let cfg = cfg_with_token("from-config");
    assert_eq!(resolve_session_token(Some(&cfg)), "from-config");
}

#[test]
fn resolve_session_token_empty_config_and_env_negative_path() {
    let _guard = EnvGuard::new();
    let cfg = cfg_with_token("");
    assert!(resolve_session_token(Some(&cfg)).is_empty());
}

#[test]
fn resolve_session_token_falls_back_to_process_env_edge_case() {
    let _guard = EnvGuard::new();
    std::env::set_var("RMIG_SESSION_TOKEN", "from-env");
    let cfg = cfg_with_token("");
    assert_eq!(resolve_session_token(Some(&cfg)), "from-env");
}

#[test]
fn resolve_session_token_config_only_when_env_unset_regression() {
    let _guard = EnvGuard::new();
    let cfg = cfg_with_token("stored-token");
    assert_eq!(
        resolve_session_token(Some(&cfg)),
        "stored-token",
        "daemon config must retain the process-env token after validation"
    );
}

#[test]
fn apply_session_token_from_config_publishes_token_for_daemon() {
    let _guard = EnvGuard::new();
    let cfg = cfg_with_token("daemon-token");
    apply_session_token_from_config(&cfg);
    // Published for later auth checks without touching the process environment
    // (no `set_var`): resolvable with no config and no env var set.
    assert_eq!(resolve_session_token(None), "daemon-token");
    assert!(std::env::var("RMIG_SESSION_TOKEN").is_err());
}

#[cfg(feature = "session-daemon")]
#[test]
fn constant_time_eq_matches_only_identical_bytes() {
    assert!(super::constant_time_eq(b"abc", b"abc"));
    assert!(!super::constant_time_eq(b"abc", b"abd"));
    assert!(!super::constant_time_eq(b"abc", b"ab"));
    assert!(super::constant_time_eq(b"", b""));
}
