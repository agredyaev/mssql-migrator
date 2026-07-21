use crate::Config;

use super::env_parse::{apply_tls, parse_bool, set_timeout};
use super::TomlConfig;

/// Builds a config with process environment taking precedence over typed TOML.
pub fn build_config(file: &TomlConfig, json_logs: bool) -> Config {
    let mut cfg = Config::default();
    let get = |name: &str, value: Option<String>| std::env::var(name).ok().or(value);
    cfg.sql_root = get("RM_SQL_ROOT", file.paths.sql_root.clone()).unwrap_or_default();
    cfg.sql_base = get("RM_SQL_BASE", file.paths.sql_base.clone()).unwrap_or_default();
    cfg.report_dir = get("RM_REPORT_DIR", file.paths.report_dir.clone()).unwrap_or_default();
    cfg.l1_cache_dir = get("RMIG_L1_CACHE_DIR", file.paths.l1_cache_dir.clone())
        .unwrap_or(cfg.l1_cache_dir.clone());
    let report_sync_default = cfg.report_sync;
    cfg.report_sync = get(
        "RM_REPORT_SYNC",
        file.execution.report_sync.map(|v| v.to_string()),
    )
    .map_or(report_sync_default, |v| parse_bool(&v));
    cfg.log_level =
        get("RM_LOG_LEVEL", file.execution.log_level.clone()).unwrap_or(cfg.log_level.clone());
    cfg.server = get("RM_DB_SERVER", file.database.server.clone()).unwrap_or_default();
    cfg.port =
        get("RM_DB_PORT", file.database.port.map(|v| v.to_string())).unwrap_or(cfg.port.clone());
    cfg.database.clear();
    cfg.db_auth = get("RM_DB_AUTH", file.database.auth.clone()).unwrap_or(cfg.db_auth.clone());
    cfg.user = std::env::var("RM_DB_USER").unwrap_or_default();
    cfg.password = std::env::var("RM_DB_PASSWORD").unwrap_or_default();
    let skip_git_default = cfg.skip_git;
    cfg.skip_git = get(
        "RM_SKIP_GIT",
        file.execution.skip_git.map(|v| v.to_string()),
    )
    .map_or(skip_git_default, |v| parse_bool(&v));
    let inspect_full_default = cfg.inspect_full;
    cfg.inspect_full = get(
        "RMIG_INSPECT_FULL",
        file.execution.inspect_full.map(|v| v.to_string()),
    )
    .map_or(inspect_full_default, |v| parse_bool(&v));
    let catalog_cache_default = cfg.catalog_cache;
    cfg.catalog_cache = get(
        "RMIG_CATALOG_CACHE",
        file.execution.catalog_cache.map(|v| v.to_string()),
    )
    .map_or(catalog_cache_default, |v| parse_bool(&v));
    let allow_adopt_default = cfg.allow_adopt;
    cfg.allow_adopt = get(
        "RMIG_ALLOW_ADOPT",
        file.execution.allow_adopt.map(|v| v.to_string()),
    )
    .map_or(allow_adopt_default, |v| parse_bool(&v));
    cfg.session_socket = get("RMIG_SESSION", file.session.socket.clone()).unwrap_or_default();
    cfg.session_token = std::env::var("RMIG_SESSION_TOKEN").unwrap_or_default();
    set_timeout(
        &get("RM_LOCK_TIMEOUT", file.execution.lock_timeout.clone()).unwrap_or_default(),
        "RM_LOCK_TIMEOUT",
        &mut cfg.lock_timeout,
    );
    set_timeout(
        &get("RM_COMMAND_TIMEOUT", file.execution.command_timeout.clone()).unwrap_or_default(),
        "RM_COMMAND_TIMEOUT",
        &mut cfg.command_timeout,
    );
    apply_tls(&mut cfg, file);
    cfg.json_logs = json_logs;
    if let Ok(n) = get(
        "RMIG_SLO_MAX_CLI_WALL_MS",
        file.execution.slo_max_cli_wall_ms.map(|v| v.to_string()),
    )
    .unwrap_or_default()
    .parse::<i64>()
    {
        if n > 0 {
            cfg.slo_max_cli_wall_ms = n;
        }
    }
    cfg
}

#[cfg(test)]
#[path = "../tests/env_build_test.rs"]
mod tests;
