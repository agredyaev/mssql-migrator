use std::sync::Mutex;

use migrator_core::config::{build_config, load_env_file};
use migrator_core::session::resolve_session_token;

static TEST_LOCK: Mutex<()> = Mutex::new(());

struct EnvGuard {
    _lock: std::sync::MutexGuard<'static, ()>,
}

impl EnvGuard {
    fn new() -> Self {
        let lock = TEST_LOCK.lock().expect("session token test lock");
        std::env::remove_var("RMIG_SESSION_TOKEN");
        Self { _lock: lock }
    }
}

impl Drop for EnvGuard {
    fn drop(&mut self) {
        std::env::remove_var("RMIG_SESSION_TOKEN");
    }
}

#[test]
fn build_config_loads_session_token_from_dotenv_happy_path() {
    let _guard = EnvGuard::new();
    let dir = tempfile::tempdir().expect("tempdir");
    let env_path = dir.path().join("session.env");
    std::fs::write(&env_path, "RMIG_SESSION_TOKEN=secret-from-file\n").expect("write env");
    let env = load_env_file(&env_path).expect("load env");
    let cfg = build_config(&env, false);
    assert_eq!(resolve_session_token(Some(&cfg)), "secret-from-file");
}

#[test]
fn build_config_without_session_token_stays_empty_negative_path() {
    let _guard = EnvGuard::new();
    let dir = tempfile::tempdir().expect("tempdir");
    let env_path = dir.path().join("session.env");
    std::fs::write(&env_path, "RM_DB_SERVER=localhost\n").expect("write env");
    let env = load_env_file(&env_path).expect("load env");
    let cfg = build_config(&env, false);
    assert!(resolve_session_token(Some(&cfg)).is_empty());
}

#[test]
fn process_env_overrides_empty_config_token_edge_case() {
    let _guard = EnvGuard::new();
    std::env::set_var("RMIG_SESSION_TOKEN", "shell-token");
    let dir = tempfile::tempdir().expect("tempdir");
    let env_path = dir.path().join("session.env");
    std::fs::write(&env_path, "RM_DB_SERVER=localhost\n").expect("write env");
    let env = load_env_file(&env_path).expect("load env");
    let cfg = build_config(&env, false);
    assert_eq!(resolve_session_token(Some(&cfg)), "shell-token");
}

#[test]
fn dotenv_token_used_when_process_env_unset_regression() {
    let _guard = EnvGuard::new();
    let dir = tempfile::tempdir().expect("tempdir");
    let env_path = dir.path().join("session.env");
    std::fs::write(
        &env_path,
        "RMIG_SESSION_TOKEN=bg011-dotenv-only\nRM_DB_SERVER=localhost\n",
    )
    .expect("write env");
    let env = load_env_file(&env_path).expect("load env");
    let cfg = build_config(&env, false);
    assert_eq!(
        resolve_session_token(Some(&cfg)),
        "bg011-dotenv-only",
        "BG-011 regression: token from --env / RMIGD_ENV must drive session auth"
    );
}
