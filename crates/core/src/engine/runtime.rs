//! Process runtime plumbing shared by the `rmig` and `rmigd` binaries.

/// Resolves when SIGINT or SIGTERM arrives; returns the signal name.
pub async fn shutdown_signal() -> &'static str {
    #[cfg(unix)]
    {
        use tokio::signal::unix::{signal, SignalKind};
        let mut term = match signal(SignalKind::terminate()) {
            Ok(s) => s,
            Err(_) => {
                return tokio::signal::ctrl_c()
                    .await
                    .map(|_| "SIGINT")
                    .unwrap_or("SIGINT")
            }
        };
        tokio::select! {
            _ = tokio::signal::ctrl_c() => "SIGINT",
            _ = term.recv() => "SIGTERM",
        }
    }
    #[cfg(not(unix))]
    {
        let _ = tokio::signal::ctrl_c().await;
        "SIGINT"
    }
}

/// Default tracing filter: a bare level is scoped to the project crates; an
/// explicit directive list (contains `=` or `,`) passes through unchanged.
pub fn default_log_filter(log_level: &str) -> String {
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
#[path = "../tests/log_filter_test.rs"]
mod log_filter_tests;
