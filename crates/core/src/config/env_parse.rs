use std::time::Duration;

use crate::error::{Error, Result};
use crate::Config;

use super::TomlConfig;

pub(super) fn parse_bool(s: &str) -> bool {
    let value = s.trim();
    value == "1"
        || ["true", "yes", "on", "y", "enabled"]
            .iter()
            .any(|expected| value.eq_ignore_ascii_case(expected))
}

fn recognized_bool(s: &str) -> bool {
    let value = s.trim();
    parse_bool(value)
        || value == "0"
        || ["false", "no", "off", "n", "disabled"]
            .iter()
            .any(|expected| value.eq_ignore_ascii_case(expected))
}

pub(super) fn validate_boolean_envs() -> Result<()> {
    const NAMES: &[&str] = &[
        "RM_REPORT_SYNC",
        "RM_SKIP_GIT",
        "RMIG_INSPECT_FULL",
        "RMIG_CATALOG_CACHE",
        "RMIG_ALLOW_ADOPT",
        "RM_DB_ENCRYPT",
        "RM_DB_TRUST_SERVER_CERTIFICATE",
    ];
    for name in NAMES {
        let Some(raw) = std::env::var_os(name) else {
            continue;
        };
        let value = raw
            .to_str()
            .ok_or_else(|| Error::Config(format!("{name} must be a UTF-8 boolean value")))?;
        validate_boolean_value(name, value)?;
    }
    Ok(())
}

fn validate_boolean_value(name: &str, value: &str) -> Result<()> {
    if recognized_bool(value) {
        return Ok(());
    }
    Err(Error::Config(format!(
        "{name} has invalid boolean value {value:?}; use true or false"
    )))
}

pub(super) fn parse_duration(s: &str) -> Result<Duration> {
    if s.is_empty() {
        return Err(Error::Config("empty duration".into()));
    }
    if let Ok(secs) = s.parse::<u64>() {
        return Ok(Duration::from_secs(secs));
    }
    let s = s.trim();
    if let Some(stripped) = s.strip_suffix('s') {
        let n: f64 = stripped.parse().map_err(|_| Error::Config(s.to_string()))?;
        return Duration::try_from_secs_f64(n).map_err(|_| Error::Config(s.to_string()));
    }
    Err(Error::Config(format!("invalid duration: {s}")))
}

pub(super) fn set_timeout(raw: &str, name: &str, slot: &mut Duration) {
    match parse_duration(raw) {
        Ok(d) => *slot = d,
        Err(_) if !raw.is_empty() => {
            tracing::warn!(var = name, value = raw, "invalid duration; keeping default")
        }
        Err(_) => {}
    }
}

pub(super) fn apply_tls(cfg: &mut Config, file: &TomlConfig) {
    let get = |name: &str, value: Option<String>| std::env::var(name).ok().or(value);
    let encrypt_default = cfg.encrypt();
    cfg.set_encrypt(
        get(
            "RM_DB_ENCRYPT",
            file.database.encrypt.map(|v| v.to_string()),
        )
        .map_or(encrypt_default, |v| parse_bool(&v)),
    );
    let trust_default = cfg.trust_server_certificate();
    cfg.set_trust_server_certificate(
        get(
            "RM_DB_TRUST_SERVER_CERTIFICATE",
            file.database
                .trust_server_certificate
                .map(|v| v.to_string()),
        )
        .map_or(trust_default, |v| parse_bool(&v)),
    );
}

#[cfg(test)]
#[path = "../tests/env_parse_test.rs"]
mod tests;
