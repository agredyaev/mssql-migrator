//! Size limits for session RPC and L1 cache (DoS hardening).

/// Max JSON line on rmigd Unix socket (single request/response).
pub const MAX_SESSION_LINE_BYTES: usize = 4 * 1024 * 1024;

/// Max concurrent rmigd client socket handlers.
pub const MAX_DAEMON_CLIENTS: usize = 64;

/// Max L1 cache JSON file size.
pub const MAX_L1_CACHE_BYTES: usize = 64 * 1024 * 1024;
