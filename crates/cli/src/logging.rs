//! CLI tracing setup.

use migrator_core::engine::default_log_filter;
use tracing_subscriber::EnvFilter;

pub(crate) fn init_tracing(log_level: &str) {
    let filter = EnvFilter::try_from_default_env()
        .or_else(|_| EnvFilter::try_new(default_log_filter(log_level)))
        .unwrap_or_else(|_| EnvFilter::new(default_log_filter("info")));

    let _ = tracing_subscriber::fmt()
        .with_env_filter(filter)
        .with_writer(std::io::stderr)
        .try_init();
}
