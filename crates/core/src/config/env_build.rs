use std::collections::HashMap;
use std::time::Duration;

use crate::error::{Error, Result};
use crate::Config;

pub(crate) fn parse_bool(s: &str) -> bool {
    matches!(
        s.trim().to_lowercase().as_str(),
        "1" | "true" | "yes" | "on" | "y" | "enabled"
    )
}

/// True when `s` is a recognizable boolean (either polarity); used to warn on
/// typo'd flags whose silent-false could disable a security control.
fn recognized_bool(s: &str) -> bool {
    let t = s.trim().to_lowercase();
    parse_bool(&t) || matches!(t.as_str(), "0" | "false" | "no" | "off" | "n" | "disabled")
}

pub(crate) fn parse_duration(s: &str) -> Result<Duration> {
    if s.is_empty() {
        return Err(Error::Config("empty duration".into()));
    }
    if let Ok(secs) = s.parse::<u64>() {
        return Ok(Duration::from_secs(secs));
    }
    let s = s.trim();
    if let Some(stripped) = s.strip_suffix('s') {
        let n: f64 = stripped.parse().map_err(|_| Error::Config(s.to_string()))?;
        // try_from_secs_f64 rejects negative/NaN/overflow instead of panicking.
        return Duration::try_from_secs_f64(n).map_err(|_| Error::Config(s.to_string()));
    }
    Err(Error::Config(format!("invalid duration: {s}")))
}

fn set_timeout(raw: &str, name: &str, slot: &mut Duration) {
    match parse_duration(raw) {
        Ok(d) => *slot = d,
        Err(_) if !raw.is_empty() => {
            tracing::warn!(var = name, value = raw, "invalid duration; keeping default")
        }
        Err(_) => {}
    }
}

/// Builds a `Config` from environment variables, consulting `env` as a fallback for each key.
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
    cfg.set_report_sync(parse_bool(&get("RM_REPORT_SYNC")));
    cfg.log_level = get("RM_LOG_LEVEL");
    cfg.server = get("RM_DB_SERVER");
    cfg.port = get("RM_DB_PORT");
    if cfg.port.is_empty() {
        cfg.port = "1433".into();
    }
    cfg.database.clear();
    cfg.db_auth = get("RM_DB_AUTH");
    cfg.user = get("RM_DB_USER");
    cfg.password = get("RM_DB_PASSWORD");
    cfg.set_skip_git(parse_bool(&get("RM_SKIP_GIT")));
    cfg.set_inspect_full(parse_bool(&get("RMIG_INSPECT_FULL")));
    cfg.set_catalog_cache(!matches!(get("RMIG_CATALOG_CACHE").as_str(), "0" | "false"));
    cfg.set_allow_adopt(parse_bool(&get("RMIG_ALLOW_ADOPT")));
    cfg.session_socket = get("RMIG_SESSION");
    cfg.session_token = get("RMIG_SESSION_TOKEN");
    set_timeout(
        &get("RM_LOCK_TIMEOUT"),
        "RM_LOCK_TIMEOUT",
        &mut cfg.lock_timeout,
    );
    set_timeout(
        &get("RM_COMMAND_TIMEOUT"),
        "RM_COMMAND_TIMEOUT",
        &mut cfg.command_timeout,
    );
    let encrypt = get("RM_DB_ENCRYPT");
    if !encrypt.is_empty() && !recognized_bool(&encrypt) {
        tracing::warn!(
            var = "RM_DB_ENCRYPT",
            value = encrypt.as_str(),
            "unrecognized boolean; treating as false (TLS disabled)"
        );
    }
    cfg.set_encrypt(parse_bool(&encrypt));
    cfg.set_trust_server_certificate(parse_bool(&get("RM_DB_TRUST_SERVER_CERTIFICATE")));
    cfg.set_json_logs(json_logs);
    if let Ok(n) = get("RMIG_SLO_MAX_CLI_WALL_MS").parse::<i64>() {
        if n > 0 {
            cfg.slo_max_cli_wall_ms = n;
        }
    }
    cfg
}
