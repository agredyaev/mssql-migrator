use std::collections::HashMap;
use std::path::Path;
use std::time::Duration;

use crate::error::{Error, Result};
use crate::Config;

#[cfg(unix)]
fn warn_env_file_permissions(path: &Path) {
    use std::os::unix::fs::PermissionsExt;
    if let Ok(meta) = std::fs::metadata(path) {
        let mode = meta.permissions().mode();
        if mode & 0o077 != 0 {
            eprintln!(
                "warning: env file {} is readable by group/other (mode {:o}); restrict to 0600",
                path.display(),
                mode & 0o777
            );
        }
    }
}

#[cfg(not(unix))]
fn warn_env_file_permissions(_path: &Path) {}

pub fn load_env_file(path: &Path) -> Result<HashMap<String, String>> {
    if path.is_file() {
        warn_env_file_permissions(path);
    }
    let Ok(content) = std::fs::read_to_string(path) else {
        return Ok(HashMap::new());
    };
    let mut env = HashMap::new();
    for line in content.lines() {
        let line = line.trim();
        if line.is_empty() || line.starts_with('#') {
            continue;
        }
        let Some((k, v)) = line.split_once('=') else {
            continue;
        };
        let v = v.trim().trim_matches(|c| c == '"' || c == '\'');
        env.insert(k.trim().to_string(), v.to_string());
    }
    Ok(env)
}

pub fn build_config(env: &HashMap<String, String>, json_logs: bool) -> Config {
    let mut cfg = Config::default();
    let get = |k: &str| -> String {
        std::env::var(k)
            .ok()
            .or_else(|| env.get(k).cloned())
            .unwrap_or_default()
    };
    cfg.sql_root = get("RM_SQL_ROOT");
    cfg.sql_base = get("RM_SQL_BASE");
    cfg.report_dir = get("RM_REPORT_DIR");
    cfg.report_sync = parse_bool(&get("RM_REPORT_SYNC"));
    cfg.log_level = get("RM_LOG_LEVEL");
    cfg.server = get("RM_DB_SERVER");
    cfg.port = get("RM_DB_PORT");
    if cfg.port.is_empty() {
        cfg.port = "1433".into();
    }
    // Database name comes from catalog layout under RM_SQL_ROOT, not env.
    cfg.database.clear();
    cfg.db_auth = get("RM_DB_AUTH");
    cfg.user = get("RM_DB_USER");
    cfg.password = get("RM_DB_PASSWORD");
    cfg.skip_git = parse_bool(&get("RM_SKIP_GIT"));
    cfg.inspect_full = parse_bool(&get("RMIG_INSPECT_FULL"));
    cfg.catalog_cache = !matches!(get("RMIG_CATALOG_CACHE").as_str(), "0" | "false");
    cfg.session_socket = get("RMIG_SESSION");
    cfg.session_token = get("RMIG_SESSION_TOKEN");
    if let Ok(d) = parse_duration(&get("RM_LOCK_TIMEOUT")) {
        cfg.lock_timeout = d;
    }
    if let Ok(d) = parse_duration(&get("RM_COMMAND_TIMEOUT")) {
        cfg.command_timeout = d;
    }
    cfg.encrypt = parse_bool(&get("RM_DB_ENCRYPT"));
    cfg.trust_server_certificate = parse_bool(&get("RM_DB_TRUST_SERVER_CERTIFICATE"));
    cfg.json_logs = json_logs;
    if let Ok(n) = get("RMIG_SLO_MAX_CLI_WALL_MS").parse::<i64>() {
        if n > 0 {
            cfg.slo_max_cli_wall_ms = n;
        }
    }
    cfg
}

fn parse_bool(s: &str) -> bool {
    matches!(s.trim().to_lowercase().as_str(), "1" | "true" | "yes")
}

fn parse_duration(s: &str) -> Result<Duration> {
    if s.is_empty() {
        return Err(Error::Config("empty duration".into()));
    }
    if let Ok(secs) = s.parse::<u64>() {
        return Ok(Duration::from_secs(secs));
    }
    let s = s.trim();
    if let Some(stripped) = s.strip_suffix('s') {
        let n: f64 = stripped.parse().map_err(|_| Error::Config(s.to_string()))?;
        return Ok(Duration::from_secs_f64(n));
    }
    Err(Error::Config(format!("invalid duration: {s}")))
}
