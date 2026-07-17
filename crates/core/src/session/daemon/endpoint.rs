//! Daemon-side SQL endpoint identity check for the session handshake.

use crate::session::protocol::{Request, Response};

/// Refusal response when `req` is a `Ping` declaring a different endpoint.
pub fn refusal_for(req: &Request, ep: &Endpoint) -> Option<Response> {
    if let Request::Ping { server, port, user } = req {
        if let Some(msg) = endpoint_mismatch(ep, server, port, user) {
            return Some(Response::err(msg));
        }
    }
    None
}

/// The SQL Server identity of the daemon's single warm connection.
pub struct Endpoint {
    pub server: String,
    pub port: String,
    pub user: String,
}

/// Returns a refusal message when a client-declared endpoint field is present
/// and differs from the daemon's. Empty client fields (legacy clients or
/// cfg-less probes) skip the comparison rather than fail it.
pub fn endpoint_mismatch(ep: &Endpoint, server: &str, port: &str, user: &str) -> Option<String> {
    let pairs = [
        ("server", server, ep.server.as_str()),
        ("port", port, ep.port.as_str()),
        ("user", user, ep.user.as_str()),
    ];
    let mismatched: Vec<&str> = pairs
        .iter()
        .filter(|(_, got, want)| !got.is_empty() && got != want)
        .map(|(name, _, _)| *name)
        .collect();
    if mismatched.is_empty() {
        return None;
    }
    Some(format!(
        "rmigd endpoint mismatch ({}): daemon serves {}:{} as {}; \
         reconnect directly or point RMIG_SESSION at a matching daemon",
        mismatched.join(", "),
        ep.server,
        ep.port,
        ep.user
    ))
}

#[cfg(test)]
#[path = "../../tests/daemon_endpoint_test.rs"]
mod daemon_endpoint_tests;
