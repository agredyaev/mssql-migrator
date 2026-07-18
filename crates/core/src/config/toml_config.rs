use std::path::Path;

use serde::Deserialize;

use crate::error::{Error, Result};

#[derive(Clone, Debug, Default, Deserialize)]
#[serde(default, deny_unknown_fields)]
/// Typed, non-secret configuration loaded from TOML.
pub struct TomlConfig {
    pub(crate) database: DatabaseConfig,
    pub(crate) paths: PathsConfig,
    pub(crate) execution: ExecutionConfig,
    pub(crate) session: SessionConfig,
}

#[derive(Clone, Debug, Default, Deserialize)]
#[serde(default, deny_unknown_fields)]
pub(crate) struct DatabaseConfig {
    pub(crate) server: Option<String>,
    pub(crate) port: Option<u16>,
    pub(crate) auth: Option<String>,
    pub(crate) encrypt: Option<bool>,
    pub(crate) trust_server_certificate: Option<bool>,
}

#[derive(Clone, Debug, Default, Deserialize)]
#[serde(default, deny_unknown_fields)]
pub(crate) struct PathsConfig {
    pub(crate) sql_root: Option<String>,
    pub(crate) sql_base: Option<String>,
    pub(crate) report_dir: Option<String>,
    pub(crate) l1_cache_dir: Option<String>,
}

#[derive(Clone, Debug, Default, Deserialize)]
#[serde(default, deny_unknown_fields)]
pub(crate) struct ExecutionConfig {
    pub(crate) log_level: Option<String>,
    pub(crate) report_sync: Option<bool>,
    pub(crate) skip_git: Option<bool>,
    pub(crate) inspect_full: Option<bool>,
    pub(crate) catalog_cache: Option<bool>,
    pub(crate) allow_adopt: Option<bool>,
    pub(crate) command_timeout: Option<String>,
    pub(crate) lock_timeout: Option<String>,
    pub(crate) slo_max_cli_wall_ms: Option<i64>,
}

#[derive(Clone, Debug, Default, Deserialize)]
#[serde(default, deny_unknown_fields)]
pub(crate) struct SessionConfig {
    pub(crate) socket: Option<String>,
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
    let text = std::fs::read_to_string(path)
        .map_err(|e| Error::Config(format!("config file unreadable: {}: {e}", path.display())))?;
    let value: toml::Value = toml::from_str(&text)
        .map_err(|e| Error::Config(format!("invalid TOML config {}: {e}", path.display())))?;
    reject_secrets(&value)?;
    value
        .try_into()
        .map_err(|e| Error::Config(format!("invalid config {}: {e}", path.display())))
}

fn reject_secrets(value: &toml::Value) -> Result<()> {
    let Some(table) = value.as_table() else {
        return Ok(());
    };
    for (section, key) in [
        ("database", "user"),
        ("database", "password"),
        ("session", "token"),
    ] {
        if table
            .get(section)
            .and_then(toml::Value::as_table)
            .is_some_and(|t| t.contains_key(key))
        {
            return Err(Error::Config(format!(
                "{section}.{key} is not allowed in TOML; set {} in the process environment",
                match key {
                    "user" => "RM_DB_USER",
                    "password" => "RM_DB_PASSWORD",
                    _ => "RMIG_SESSION_TOKEN",
                }
            )));
        }
    }
    Ok(())
}

#[cfg(test)]
#[path = "../tests/toml_config_test.rs"]
mod tests;
