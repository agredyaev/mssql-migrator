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

pub use auth::session_token_from_env;
#[cfg(feature = "session-daemon")]
pub use auth::{token_required, verify_token};
pub use client::connect_daemon;
pub use proxy::ProxyClient;
pub use socket::{default_socket_path, resolve_socket_path};

#[cfg(feature = "session-daemon")]
pub use daemon::run_daemon;
