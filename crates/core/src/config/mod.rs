//! Environment variables, dotenv parsing, and run-time config parameters.
//!
//! ### Purpose
//! Resolves, validates, and stores database connection settings, SQL layout paths, cache policies,
//! and threshold limits from system environment variables or dynamic `.env` configurations.
//!
//! ### Architectural Context
//! - **Inputs**: Local process environment, target `.env` files.
//! - **Outputs**: Parsed `Config` structs (containing hot run parameters and cold connection data in `ConfigCold`).
//! - **Boundaries**: Connection settings are kept read-only behind `Arc` thread-safety wrappers.
//!
//! ### Nominal Flow
//! 1. Discover and load environment structures (`load_env_file`).
//! 2. Retrieve variables, checking against defaults (`build_config`).
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
mod env;
mod env_build;
mod flags;
mod validate;

use std::ops::{Deref, DerefMut};
use std::sync::Arc;

pub use auth_mode::{sql_credentials_required, uses_integrated_auth};
pub use catalog::discover_catalog_databases;
pub use catalog_paths::{normalize_catalog_paths, resolve_single_database};
pub use cold::ConfigCold;
pub use ensure_db::{ensure_catalog_databases_exist, target_database_exists};
pub use env::{load_env_file, load_env_file_required};
pub use env_build::build_config;
pub use validate::validate_config;

/// Hot run flags + layout paths; connection block in [`ConfigCold`] behind `Arc`.
#[derive(Clone)]
pub struct Config {
    pub sql_root: String,
    pub sql_base: String,
    pub report_dir: String,
    pub log_level: String,
    pub database: String,
    pub slo_max_cli_wall_ms: i64,
    pub(crate) cold: Arc<ConfigCold>,
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
