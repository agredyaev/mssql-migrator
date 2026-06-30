//! SQL Server authentication method selection from [`Config`](crate::config).

use tiberius::AuthMethod;

use crate::config::Config;
use crate::error::{Error, Result};

/// Map `RM_DB_AUTH` to the TDS authentication method used by `connect`.
pub fn select_auth_method(cfg: &Config) -> Result<AuthMethod> {
    let mode = cfg.db_auth.to_ascii_lowercase();
    match mode.as_str() {
        "" | "sql" => Ok(AuthMethod::sql_server(&cfg.user, &cfg.password)),
        "integrated" | "windows" => Err(Error::Config(
            "integrated auth not compiled; enable tiberius features: \
             integrated-auth-gssapi (Unix) or winauth (Windows)"
                .into(),
        )),
        other => Err(Error::Config(format!(
            "unsupported RM_DB_AUTH value: {other}"
        ))),
    }
}
