//! Typed TOML, environment-only secrets, and run-time config parameters.
//!
//! ### Purpose
//! Resolves, validates, and stores database connection settings, SQL layout paths, cache policies,
//! and threshold limits from typed TOML plus process environment overrides.
//!
//! ### Architectural Context
//! - **Inputs**: Local process environment and a typed TOML file.
//! - **Outputs**: Parsed `Config` structs (containing hot run parameters and cold connection data in `ConfigCold`).
//! - **Boundaries**: Connection settings are kept read-only behind `Arc` thread-safety wrappers.
//!
//! ### Nominal Flow
//! 1. Load typed TOML (`load_toml_config`).
//! 2. Apply environment overrides and defaults (`build_config`).
//! 3. Perform sanity checks on paths and servers (`validate_config`).
//!
//! ### Off-Nominal & Failure Containment
//! - **Missing Variables / Bad Formats**: Halts processing, formats error outputs, and returns `Error::Config`.

mod accessors;
mod auth_mode;
mod catalog;
mod catalog_paths;
mod cold;
mod debug;
mod default;
mod ensure_db;
mod env_build;
mod env_parse;
mod flags;
mod toml_config;
mod validate;

use std::ops::{Deref, DerefMut};
use std::sync::Arc;

pub use auth_mode::sql_credentials_required;
pub use catalog::discover_catalog_databases;
pub use catalog_paths::{normalize_catalog_paths, resolve_single_database};
pub use cold::ConfigCold;
pub use ensure_db::{ensure_catalog_databases_exist, target_database_exists};
pub use env_build::build_config;
pub use toml_config::{load_toml_config, load_toml_config_required, TomlConfig};
pub use validate::{validate_config, validate_daemon_config};

/// Hot run flags + layout paths; connection block in [`ConfigCold`] behind `Arc`.
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
    /// Wall-clock SLO ceiling for CLI runs, in milliseconds.
    pub slo_max_cli_wall_ms: i64,
    pub(crate) cold: Arc<ConfigCold>,
    /// Bitfield of run-mode flags.
    pub flags: u8,
}

impl Deref for Config {
    type Target = ConfigCold;
    fn deref(&self) -> &ConfigCold {
        &self.cold
    }
}

impl DerefMut for Config {
    fn deref_mut(&mut self) -> &mut ConfigCold {
        Arc::make_mut(&mut self.cold)
    }
}
