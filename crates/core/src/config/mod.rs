mod accessors;
mod catalog;
mod cold;
mod debug;
mod default;
mod env;
mod flags;
mod validate;

use std::ops::{Deref, DerefMut};
use std::sync::Arc;

pub use catalog::{
    discover_catalog_databases, ensure_catalog_databases_exist, normalize_catalog_paths,
    resolve_single_database,
};
pub use cold::ConfigCold;
pub use env::{build_config, load_env_file};
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
