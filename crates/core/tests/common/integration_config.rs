//! Parity / SLO integration config (used via `common/mod.rs` only).

use std::path::Path;

use migrator_core::config::{build_config, load_env_file, validate_config};
use migrator_core::Config;

pub fn repo_root() -> std::path::PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("../..")
        .canonicalize()
        .expect("repo root")
}

/// Base config for integration / SLO tests (`skip_git`). Caller may attach `rmigd` socket.
pub fn parity_config_base() -> Config {
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
    cfg.set_skip_git(true);
    cfg.set_trust_server_certificate(true);
    cfg.set_catalog_cache(!matches!(
        std::env::var("RMIG_CATALOG_CACHE").as_deref(),
        Ok("0") | Ok("false")
    ));
    if let Some(n) = std::env::var("RMIG_INTEGRATION_SLO_MS")
        .ok()
        .and_then(|s| s.parse().ok())
    {
        cfg.slo_max_cli_wall_ms = n;
    }
    validate_config(&mut cfg).expect("valid parity config");
    cfg
}
