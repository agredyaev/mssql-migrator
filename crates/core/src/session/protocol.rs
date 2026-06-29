use serde::{Deserialize, Serialize};

use crate::driver::RowData;

#[derive(Serialize, Deserialize)]
#[serde(tag = "op", rename_all = "snake_case")]
pub enum Request {
    /// First message when `RMIG_SESSION_TOKEN` is set on daemon and client.
    Auth {
        token: String,
    },
    Ping,
    Exec {
        sql: String,
    },
    Query {
        sql: String,
        params: Vec<String>,
    },
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
            Self::Ping => f.write_str("Ping"),
            Self::Exec { sql } => f.debug_struct("Exec").field("sql", sql).finish(),
            Self::Query { sql, params } => f
                .debug_struct("Query")
                .field("sql", sql)
                .field("params", params)
                .finish(),
        }
    }
}

#[derive(Debug, Serialize, Deserialize)]
pub struct Response {
    pub ok: bool,
    #[serde(default)]
    pub error: String,
    #[serde(default)]
    pub rows: Vec<RowData>,
}

impl Response {
    pub fn ok_empty() -> Self {
        Self {
            ok: true,
            error: String::new(),
            rows: Vec::new(),
        }
    }

    pub fn err(msg: impl Into<String>) -> Self {
        Self {
            ok: false,
            error: msg.into(),
            rows: Vec::new(),
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
