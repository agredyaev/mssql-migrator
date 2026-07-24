//! Session RPC size and concurrency limits.

/// Max JSON line on rmigd Unix socket (single request/response).
pub const MAX_SESSION_LINE_BYTES: usize = 4 * 1024 * 1024;

/// Max concurrent rmigd client socket handlers.
pub const MAX_DAEMON_CLIENTS: usize = 64;
