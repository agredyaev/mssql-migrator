use std::path::Path;

use serde::Deserialize;

use crate::error::{Error, Result};
use crate::file_io::MAX_CONFIG_BYTES;

#[derive(Clone, Debug, Default, Deserialize)]
#[serde(default, deny_unknown_fields)]
/// Typed, non-secret configuration loaded from TOML.
pub struct TomlConfig {
    pub(crate) paths: PathsConfig,
    pub(crate) execution: ExecutionConfig,
}

#[derive(Clone, Debug, Default, Deserialize)]
#[serde(default, deny_unknown_fields)]
pub(crate) struct PathsConfig {
    pub(crate) sql_root: Option<String>,
    pub(crate) sql_base: Option<String>,
    pub(crate) report_dir: Option<String>,
}

#[derive(Clone, Debug, Default, Deserialize)]
#[serde(default, deny_unknown_fields)]
pub(crate) struct ExecutionConfig {
    pub(crate) log_level: Option<String>,
    pub(crate) report_sync: Option<bool>,
    pub(crate) skip_git: Option<bool>,
    pub(crate) inspect_full: Option<bool>,
    pub(crate) catalog_cache: Option<bool>,
    pub(crate) command_timeout: Option<String>,
    pub(crate) lock_timeout: Option<String>,
}

/// Loads optional typed TOML; a missing default file means env-only config.
pub fn load_toml_config(path: &Path) -> Result<TomlConfig> {
    load_toml_config_inner(path, false)
}

/// Loads an explicitly requested TOML file and rejects a missing path.
pub fn load_toml_config_required(path: &Path) -> Result<TomlConfig> {
    load_toml_config_inner(path, true)
}

fn load_toml_config_inner(path: &Path, required: bool) -> Result<TomlConfig> {
    if !path.exists() && !required {
        return Ok(TomlConfig::default());
    }
    let data = crate::file_io::read_bounded(path, MAX_CONFIG_BYTES)
        .map_err(|e| Error::Config(format!("config file unreadable: {}: {e}", path.display())))?;
    let text = String::from_utf8(data).map_err(|_| {
        Error::Config(format!(
            "config file is not valid UTF-8: {}",
            path.display()
        ))
    })?;
    let value: toml::Value = toml::from_str(&text).map_err(|e| {
        Error::Config(format!(
            "invalid TOML config {}: {}",
            path.display(),
            e.message()
        ))
    })?;
    reject_environment_only(&value)?;
    value.try_into().map_err(|e: toml::de::Error| {
        Error::Config(format!(
            "invalid config {}: {}",
            path.display(),
            e.message()
        ))
    })
}

fn reject_environment_only(value: &toml::Value) -> Result<()> {
    let Some(table) = value.as_table() else {
        return Ok(());
    };
    for (section, key, variable) in [
        ("database", "server", "RM_DB_SERVER"),
        ("database", "port", "RM_DB_PORT"),
        ("database", "encrypt", "RM_DB_ENCRYPT"),
        (
            "database",
            "trust_server_certificate",
            "RM_DB_TRUST_SERVER_CERTIFICATE",
        ),
        ("database", "user", "RM_DB_USER"),
        ("database", "password", "RM_DB_PASSWORD"),
        ("session", "socket", "RMIG_SESSION"),
        ("session", "token", "RMIG_SESSION_TOKEN"),
        ("execution", "allow_adopt", "RMIG_ALLOW_ADOPT"),
    ] {
        if table
            .get(section)
            .and_then(toml::Value::as_table)
            .is_some_and(|t| t.contains_key(key))
        {
            return Err(Error::Config(format!(
                "{section}.{key} is not allowed in TOML; set {variable} in the process environment"
            )));
        }
    }
    Ok(())
}

#[cfg(test)]
#[path = "../tests/toml_config_test.rs"]
mod tests;
