//! CLI tracing setup.

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

fn default_log_filter(log_level: &str) -> String {
    let level = if log_level.trim().is_empty() {
        "info"
    } else {
        log_level.trim()
    };
    if level.contains('=') || level.contains(',') {
        level.to_string()
    } else {
        format!("warn,migrator_core={level},rmig={level},rmigd={level}")
    }
}

#[cfg(test)]
#[path = "tests/logging_test.rs"]
mod logging_tests;
