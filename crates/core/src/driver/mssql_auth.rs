//! SQL Server authentication method selection from [`Config`](crate::config).

use tiberius::AuthMethod;

use crate::config::Config;
use crate::error::{Error, Result};

/// Map `RM_DB_AUTH` to the TDS authentication method used by `connect`.
pub fn select_auth_method(cfg: &Config) -> Result<AuthMethod> {
    let mode = cfg.db_auth.to_ascii_lowercase();
    match mode.as_str() {
        "" | "sql" => Ok(AuthMethod::sql_server(&cfg.user, &cfg.password)),
        "integrated" => Ok(AuthMethod::Integrated),
        "windows" if cfg.user.is_empty() && cfg.password.is_empty() => Ok(AuthMethod::Integrated),
        "windows" => {
            #[cfg(windows)]
            {
                Ok(AuthMethod::windows(&cfg.user, &cfg.password))
            }
            #[cfg(not(windows))]
            {
                Err(Error::Config(
                    "RM_DB_AUTH=windows with RM_DB_USER/RM_DB_PASSWORD requires a Windows host"
                        .into(),
                ))
            }
        }
        other => Err(Error::Config(format!(
            "unsupported RM_DB_AUTH value: {other}"
        ))),
    }
}
