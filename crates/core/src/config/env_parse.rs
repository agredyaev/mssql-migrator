use std::time::Duration;

use crate::error::{Error, Result};
use crate::Config;

use super::TomlConfig;

pub(super) fn parse_bool(s: &str) -> bool {
    matches!(
        s.trim().to_lowercase().as_str(),
        "1" | "true" | "yes" | "on" | "y" | "enabled"
    )
}

fn recognized_bool(s: &str) -> bool {
    let t = s.trim().to_lowercase();
    parse_bool(&t) || matches!(t.as_str(), "0" | "false" | "no" | "off" | "n" | "disabled")
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
    let encrypt = get(
        "RM_DB_ENCRYPT",
        file.database.encrypt.map(|v| v.to_string()),
    )
    .unwrap_or_default();
    if !encrypt.is_empty() && !recognized_bool(&encrypt) {
        tracing::warn!(
            var = "RM_DB_ENCRYPT",
            value = encrypt.as_str(),
            "unrecognized boolean; treating as false (TLS disabled)"
        );
    }
    cfg.set_encrypt(parse_bool(&encrypt));
    let default = cfg.trust_server_certificate();
    cfg.set_trust_server_certificate(
        get(
            "RM_DB_TRUST_SERVER_CERTIFICATE",
            file.database
                .trust_server_certificate
                .map(|v| v.to_string()),
        )
        .map_or(default, |v| parse_bool(&v)),
    );
}
