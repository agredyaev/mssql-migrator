#![allow(missing_docs)]

use serde::{Deserialize, Serialize};

use crate::config::Config;
use crate::driver::RowData;

#[derive(Serialize, Deserialize)]
#[serde(tag = "op", rename_all = "snake_case")]
pub enum Request {
    /// First message when `RMIG_SESSION_TOKEN` is set on daemon and client.
    Auth {
        token: String,
    },
    /// Handshake. The endpoint fields carry the client's configured SQL Server
    /// identity so the daemon can refuse a session whose warm connection points
    /// at a different server/port/login/TLS policy. Empty/absent fields mean a
    /// legacy client.
    Ping {
        #[serde(default)]
        server: String,
        #[serde(default)]
        port: String,
        #[serde(default)]
        user: String,
        #[serde(default)]
        encrypt: Option<bool>,
        #[serde(default)]
        trust_server_certificate: Option<bool>,
    },
    Exec {
        sql: String,
    },
    Query {
        sql: String,
        params: Vec<String>,
    },
    /// Daemon metrics/health pull: returns counters and warm-connection state in
    /// `Response.stats`. Handled at the daemon level; does not touch SQL Server.
    Stats {},
}

impl Request {
    pub(super) fn ping(cfg: &Config) -> Self {
        Self::Ping {
            server: cfg.server.clone(),
            port: cfg.port.clone(),
            user: cfg.user.clone(),
            encrypt: Some(cfg.encrypt),
            trust_server_certificate: Some(cfg.trust_server_certificate),
        }
    }
}

/// `Debug` is hand-written (not derived) so the `Auth` session token is never
/// printed in panic backtraces or diagnostics; see the redacting `Debug` on
/// `config/cold.rs`. SQL text stays visible — it is repository content, not a secret.
impl std::fmt::Debug for Request {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Auth { .. } => f
                .debug_struct("Auth")
                .field("token", &"<redacted>")
                .finish(),
            Self::Ping { server, port, .. } => f
                .debug_struct("Ping")
                .field("server", server)
                .field("port", port)
                .finish(),
            Self::Exec { sql } => f.debug_struct("Exec").field("sql", sql).finish(),
            Self::Query { sql, params } => f
                .debug_struct("Query")
                .field("sql", sql)
                .field("params", params)
                .finish(),
            Self::Stats {} => f.debug_struct("Stats").finish(),
        }
    }
}

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct Response {
    pub ok: bool,
    #[serde(default)]
    pub error: String,
    #[serde(default)]
    pub rows: Vec<RowData>,
    /// JSON metrics blob for a `Stats` request; empty for all other responses.
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub stats: String,
}

impl Response {
    pub fn ok_empty() -> Self {
        Self {
            ok: true,
            ..Self::default()
        }
    }

    pub fn err(msg: impl Into<String>) -> Self {
        Self {
            error: msg.into(),
            ..Self::default()
        }
    }

    /// Response carrying a daemon metrics JSON blob (see `daemon::metrics`).
    pub fn stats(json: String) -> Self {
        Self {
            ok: true,
            stats: json,
            ..Self::default()
        }
    }

    pub fn into_result(self) -> crate::error::Result<Vec<RowData>> {
        if self.ok {
            Ok(self.rows)
        } else {
            Err(crate::error::Error::Sql(self.error))
        }
    }
}

#[cfg(test)]
#[path = "../tests/protocol_test.rs"]
mod protocol_tests;
