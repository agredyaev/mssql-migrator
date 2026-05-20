use serde::{Deserialize, Serialize};

use crate::driver::RowData;

#[derive(Debug, Serialize, Deserialize)]
#[serde(tag = "op", rename_all = "snake_case")]
pub enum Request {
    /// First message when `RMIG_SESSION_TOKEN` is set on daemon and client.
    Auth { token: String },
    Ping,
    Exec { sql: String },
    Query { sql: String, params: Vec<String> },
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
