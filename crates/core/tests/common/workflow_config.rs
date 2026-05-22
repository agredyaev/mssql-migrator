//! Workflow / apply e2e config (`skip_git` off by default; catalog DB from layout).

use std::path::Path;
use std::sync::OnceLock;

use migrator_core::config::{build_config, load_env_file, validate_config};
use migrator_core::Config;

static WORKFLOW_CFG: OnceLock<Config> = OnceLock::new();

fn repo_root() -> std::path::PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("../..")
        .canonicalize()
        .expect("repo root")
}

pub fn workflow_config() -> &'static Config {
    WORKFLOW_CFG.get_or_init(|| {
        let env = load_env_file(&repo_root().join(".env")).unwrap_or_default();
        let mut cfg = build_config(&env, true);
        if cfg.server.is_empty() {
            cfg.server = "127.0.0.1".into();
        }
        if cfg.port.is_empty() {
            cfg.port = "1433".into();
        }
        if cfg.user.is_empty() {
            cfg.user = "sa".into();
        }
        if cfg.password.is_empty() {
            cfg.password = "yourStrong(!)Password".into();
        }
        let sql = repo_root().join(".temp/sql");
        cfg.sql_root = sql.to_string_lossy().into();
        if cfg.sql_base.is_empty() {
            cfg.sql_base = cfg.sql_root.clone();
        }
        cfg.set_skip_git(false);
        cfg.session_socket.clear();
        cfg.set_trust_server_certificate(true);
        cfg.set_catalog_cache(true);
        validate_config(&mut cfg).expect("valid workflow config");
        cfg
    })
}
