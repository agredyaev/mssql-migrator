//! Typed TOML, environment-only connection settings and secrets, and run-time parameters.
//!
//! ### Purpose
//! Resolves, validates, and stores environment-provided database connection settings plus typed
//! TOML layout paths, execution flags, and timeout limits.
//!
//! ### Architectural Context
//! - **Inputs**: Local process environment and a typed TOML file.
//! - **Outputs**: Parsed `Config` values.
//! - **Boundaries**: Connection settings and secrets come from the process environment.
//!
//! ### Nominal Flow
//! 1. Load typed TOML (`load_toml_config`).
//! 2. Apply environment-only peer settings, overrides, and defaults (`build_config`).
//! 3. Perform sanity checks on paths and servers (`validate_config`).
//!
//! ### Off-Nominal & Failure Containment
//! - **Missing Variables / Bad Formats**: Halts processing, formats error outputs, and returns `Error::Config`.

mod catalog;
mod debug;
mod default;
mod ensure_db;
mod env_build;
mod env_parse;
mod toml_config;
mod validate;

use std::time::Duration;

pub use catalog::discover_catalog_databases;
pub use ensure_db::{ensure_catalog_databases_exist, target_database_exists};
pub use env_build::build_config;
pub use toml_config::{load_toml_config, load_toml_config_required, TomlConfig};
pub use validate::{validate_config, validate_daemon_config};

/// Runtime configuration assembled from TOML and process environment values.
#[derive(Clone)]
pub struct Config {
    /// Root directory of the SQL source tree.
    pub sql_root: String,
    /// Base SQL layout path used for diff comparisons.
    pub sql_base: String,
    /// Directory where run reports are written.
    pub report_dir: String,
    /// Active log verbosity level.
    pub log_level: String,
    /// Target database name.
    pub database: String,
    /// SQL Server hostname or IP.
    pub server: String,
    /// SQL Server port.
    pub port: String,
    /// SQL login user.
    pub user: String,
    /// SQL login password.
    pub password: String,
    /// Unix socket path for `rmigd`.
    pub session_socket: String,
    /// Session token for daemon connections.
    pub session_token: String,
    /// Per-command execution timeout for TDS queries.
    pub command_timeout: Duration,
    /// Advisory-lock acquire timeout.
    pub lock_timeout: Duration,
    /// Whether TDS encryption is required.
    pub encrypt: bool,
    /// Whether the SQL Server certificate is trusted without validation.
    pub trust_server_certificate: bool,
    /// Whether run reports should be written.
    pub report_sync: bool,
    /// Whether git operations should be skipped.
    pub skip_git: bool,
    /// Whether full-inspect mode is enabled.
    pub inspect_full: bool,
    /// Whether the catalog cache is enabled.
    pub catalog_cache: bool,
    /// Whether `migrate` may adopt existing unrecorded objects (`RMIG_ALLOW_ADOPT`).
    /// Adoption trusts live objects by name alone, so it requires explicit opt-in.
    pub allow_adopt: bool,
}
