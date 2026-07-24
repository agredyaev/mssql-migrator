use std::path::Path;
use std::sync::{Mutex, MutexGuard, OnceLock};

use migrator_core::config::{build_config, load_toml_config_required, validate_config};

const CONFIG_ENV_KEYS: &[&str] = &[
    "RM_SQL_ROOT",
    "RM_SQL_BASE",
    "RM_REPORT_DIR",
    "RM_REPORT_SYNC",
    "RM_LOG_LEVEL",
    "RM_DB_SERVER",
    "RM_DB_PORT",
    "RM_DB_DATABASE",
    "RM_DB_USER",
    "RM_DB_PASSWORD",
    "RM_SKIP_GIT",
    "RMIG_INSPECT_FULL",
    "RMIG_CATALOG_CACHE",
    "RMIG_ALLOW_ADOPT",
    "RMIG_SESSION",
    "RMIG_SESSION_TOKEN",
    "RM_LOCK_TIMEOUT",
    "RM_COMMAND_TIMEOUT",
    "RM_DB_ENCRYPT",
    "RM_DB_TRUST_SERVER_CERTIFICATE",
];
static CONFIG_ENV_LOCK: OnceLock<Mutex<()>> = OnceLock::new();

struct IsolatedConfigEnv {
    _lock: MutexGuard<'static, ()>,
    saved: Vec<(&'static str, Option<String>)>,
}

impl IsolatedConfigEnv {
    fn new() -> Self {
        let lock = CONFIG_ENV_LOCK
            .get_or_init(|| Mutex::new(()))
            .lock()
            .expect("config env lock");
        let saved = CONFIG_ENV_KEYS
            .iter()
            .map(|&key| {
                let value = std::env::var(key).ok();
                std::env::remove_var(key);
                (key, value)
            })
            .collect();
        std::env::set_var("RM_DB_USER", "sa");
        std::env::set_var("RM_DB_PASSWORD", "secret");
        Self { _lock: lock, saved }
    }
}

impl Drop for IsolatedConfigEnv {
    fn drop(&mut self) {
        for (key, value) in self.saved.drain(..) {
            match value {
                Some(value) => std::env::set_var(key, value),
                None => std::env::remove_var(key),
            }
        }
    }
}

fn write_single_db_layout(root: &Path, db: &str) {
    std::fs::create_dir_all(root.join(format!("{db}/smoke/tables"))).expect("mkdir layout");
    std::fs::write(
        root.join(format!("{db}/smoke/tables/t1.sql")),
        "CREATE TABLE smoke.t1 (id INT NOT NULL);\n",
    )
    .expect("write sql");
}

fn write_multi_db_layout(root: &Path) {
    write_single_db_layout(root, "dactests");
    std::fs::create_dir_all(root.join("warehouse/reporting/views")).expect("mkdir warehouse");
    std::fs::write(
        root.join("warehouse/reporting/views/v1.sql"),
        "CREATE VIEW reporting.v1 AS SELECT 1 AS n;\n",
    )
    .expect("write view sql");
}

fn config_for(root: &Path) -> migrator_core::Config {
    let config_path = root.join("test-config.toml");
    let sql_root = root.to_string_lossy();
    std::env::set_var("RM_DB_SERVER", "localhost");
    std::fs::write(&config_path, format!("[paths]\nsql_root = {sql_root:?}\n"))
        .expect("write config");
    let file = load_toml_config_required(&config_path).expect("load config");
    build_config(&file)
}

#[test]
fn validate_config_derives_database_from_sql_root_happy_path() {
    let _env = IsolatedConfigEnv::new();
    let dir = tempfile::tempdir().expect("tempdir");
    write_single_db_layout(dir.path(), "dactests");
    let mut cfg = config_for(dir.path());
    validate_config(&mut cfg).expect("valid config");
    assert_eq!(cfg.database, "dactests");
}

#[test]
fn rm_db_database_env_does_not_override_catalog_negative_path() {
    let _env = IsolatedConfigEnv::new();
    let dir = tempfile::tempdir().expect("tempdir");
    write_single_db_layout(dir.path(), "dactests");
    std::env::set_var("RM_DB_DATABASE", "wrongdb");
    let mut cfg = config_for(dir.path());
    validate_config(&mut cfg).expect("valid config");
    assert_ne!(cfg.database, "wrongdb");
    assert_eq!(cfg.database, "dactests");
}

#[test]
fn multi_database_layout_leaves_database_empty_edge_case() {
    let _env = IsolatedConfigEnv::new();
    let dir = tempfile::tempdir().expect("tempdir");
    write_multi_db_layout(dir.path());
    let mut cfg = config_for(dir.path());
    validate_config(&mut cfg).expect("multi-db layout validates");
    assert!(
        cfg.database.is_empty(),
        "engine discovers per-db targets from RM_SQL_ROOT, not RM_DB_DATABASE"
    );
}

#[test]
fn rm_db_database_mismatch_with_sql_root_regression() {
    let _env = IsolatedConfigEnv::new();
    let dir = tempfile::tempdir().expect("tempdir");
    write_single_db_layout(dir.path(), "dactests");
    std::env::set_var("RM_DB_DATABASE", "warehouse");
    let mut cfg = config_for(dir.path());
    validate_config(&mut cfg).expect("BG-014 regression: rmig must not honor RM_DB_DATABASE");
    assert_eq!(
        cfg.database, "dactests",
        "BG-014 regression: target DB comes from catalog layout, not RM_DB_DATABASE"
    );
}

/// An unreadable candidate directory must error out, never silently shrink
/// the deployment scope to the readable subset.
#[cfg(unix)]
#[test]
fn discover_errors_on_unreadable_candidate_dir_negative_path() {
    use std::os::unix::fs::PermissionsExt;
    let dir = tempfile::tempdir().expect("tempdir");
    write_single_db_layout(dir.path(), "dactests");
    let locked = dir.path().join("locked_db");
    std::fs::create_dir(&locked).expect("mkdir");
    std::fs::set_permissions(&locked, std::fs::Permissions::from_mode(0o000)).expect("chmod");

    let err =
        migrator_core::config::discover_catalog_databases(dir.path().to_str().expect("utf8 path"))
            .expect_err("unreadable candidate must error, not silently shrink scope");

    std::fs::set_permissions(&locked, std::fs::Permissions::from_mode(0o755)).expect("restore");
    assert!(
        err.to_string().contains("cannot read catalog candidate"),
        "got: {err}"
    );
}

#[cfg(target_os = "linux")]
#[test]
fn discover_rejects_non_utf8_catalog_name_regression() {
    use std::os::unix::ffi::OsStringExt;

    let dir = tempfile::tempdir().expect("tempdir");
    let name = std::ffi::OsString::from_vec(b"bad\xff".to_vec());
    std::fs::create_dir_all(dir.path().join(name).join("smoke")).expect("mkdir");
    let err =
        migrator_core::config::discover_catalog_databases(dir.path().to_str().expect("utf8 root"))
            .expect_err("non-UTF-8 catalog name must fail");
    assert!(err.to_string().contains("not valid UTF-8"), "{err}");
}
