//! RPC session client/daemon connection proxying and authentication protocol.
//!
//! ### Purpose
//! Manages warm connection multiplexing over Unix domain sockets between the CLI client and the `rmigd` session daemon,
//! bypassing heavy TLS/TDS connection handshakes under high-frequency executions.
//!
//! ### Architectural Context
//! - **Inputs**: Unix socket messages, token environment configurations.
//! - **Outputs**: Proxy connections, validated session tokens.
//! - **Boundaries**: Operates strictly via Unix domain sockets to isolate process communication.
//!
//! ### Nominal Flow
//! 1. Client resolves UDS socket location (`resolve_socket_path`).
//! 2. Authenticate session handshake using shared environment tokens (`verify_token`).
//! 3. Exchange schema query packages using custom session protocol structures (`limits`).
//! 4. Process queries through the multiplexed proxy client.
//!
//! ### Off-Nominal & Failure Containment
//! - **Daemon Crashing / Socket Unreachable**: If the domain socket is missing or fails, the client automatically falls back safely to direct, standard TCP database connections, logging the event to stderr.

mod auth;
mod client;
pub mod limits;
pub mod protocol;
mod proxy;
pub mod socket;

#[cfg(feature = "session-daemon")]
mod daemon;
#[cfg(feature = "session-daemon")]
mod daemon_rpc;

pub use auth::{apply_session_token_from_config, resolve_session_token};
pub use client::{connect_daemon, connect_session_or_direct};
pub use proxy::ProxyClient;
pub use socket::{default_socket_path, resolve_socket_path};

#[cfg(feature = "session-daemon")]
pub use daemon::run_daemon;
