//! [`ConfigCold`] — connection, session, and cache paths.
//!
//! ### Purpose
//! Holds the rarely-changed config fields (server, port, credentials, socket,
//! cache dir, timeouts) that are `Arc`-cloned alongside [`super::Config`].
//! Boolean flags (encrypt, trust-server-certificate) are packed into a `u8`
//! bit-field for cache efficiency.

use std::time::Duration;

use super::flags::{
    flag_get, flag_set, CONFIG_COLD_FLAG_ENCRYPT, CONFIG_COLD_FLAG_TRUST_SERVER_CERTIFICATE,
};

/// Connection, session, and cache paths. Cheap `Arc` clone with [`super::Config`].
///
/// `Debug` is implemented by hand (not derived) so `password` and `session_token`
/// are never printed; see [`super::Config`]'s redacting `Debug` in `config/debug.rs`.
#[derive(Clone, Default)]
pub struct ConfigCold {
    /// SQL Server hostname or IP.
    pub server: String,
    /// SQL Server port.
    pub port: String,
    /// Database authentication database.
    pub db_auth: String,
    /// SQL login user.
    pub user: String,
    /// SQL login password.
    pub password: String,
    /// Unix socket path for `rmigd` session daemon.
    pub session_socket: String,
    /// Session token for daemon connection.
    pub session_token: String,
    /// Directory for the L1 filesystem plan cache.
    pub l1_cache_dir: String,
    /// Per-command execution timeout for TDS queries.
    pub command_timeout: Duration,
    /// Advisory-lock acquire timeout.
    pub lock_timeout: Duration,
    /// Bit-field: CONFIG_COLD_FLAG_ENCRYPT | CONFIG_COLD_FLAG_TRUST_SERVER_CERTIFICATE.
    pub cold_flags: u8,
}

impl std::fmt::Debug for ConfigCold {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        let token = if self.session_token.is_empty() {
            "<unset>"
        } else {
            "<redacted>"
        };
        f.debug_struct("ConfigCold")
            .field("server", &self.server)
            .field("port", &self.port)
            .field("db_auth", &self.db_auth)
            .field("user", &self.user)
            .field("password", &"<redacted>")
            .field("session_socket", &self.session_socket)
            .field("session_token", &token)
            .field("l1_cache_dir", &self.l1_cache_dir)
            .field("command_timeout", &self.command_timeout)
            .field("lock_timeout", &self.lock_timeout)
            .field("cold_flags", &self.cold_flags)
            .finish()
    }
}

impl ConfigCold {
    /// True when `Encrypt=yes` is set in the connection string.
    #[inline]
    pub fn encrypt(&self) -> bool {
        flag_get(self.cold_flags, CONFIG_COLD_FLAG_ENCRYPT)
    }

    /// Set the encrypt flag.
    #[inline]
    pub fn set_encrypt(&mut self, on: bool) {
        flag_set(&mut self.cold_flags, CONFIG_COLD_FLAG_ENCRYPT, on);
    }

    /// True when `TrustServerCertificate=yes` is set.
    #[inline]
    pub fn trust_server_certificate(&self) -> bool {
        flag_get(self.cold_flags, CONFIG_COLD_FLAG_TRUST_SERVER_CERTIFICATE)
    }

    /// Set the trust-server-certificate flag.
    #[inline]
    pub fn set_trust_server_certificate(&mut self, on: bool) {
        flag_set(
            &mut self.cold_flags,
            CONFIG_COLD_FLAG_TRUST_SERVER_CERTIFICATE,
            on,
        );
    }
}

#[cfg(test)]
mod tests {
    use super::ConfigCold;

    #[test]
    fn debug_redacts_password_and_token() {
        let cold = ConfigCold {
            server: "db.example.com".into(),
            user: "sa".into(),
            password: "s3cr3t-pw".into(),
            session_token: "tok-abc123".into(),
            ..ConfigCold::default()
        };
        let dbg = format!("{cold:?}");
        assert!(!dbg.contains("s3cr3t-pw"), "password leaked: {dbg}");
        assert!(!dbg.contains("tok-abc123"), "token leaked: {dbg}");
        assert!(
            dbg.contains("<redacted>"),
            "expected redaction marker: {dbg}"
        );
        // Non-secret fields stay visible for diagnostics.
        assert!(
            dbg.contains("db.example.com"),
            "server should be visible: {dbg}"
        );
    }

    #[test]
    fn debug_marks_unset_token() {
        let dbg = format!("{:?}", ConfigCold::default());
        assert!(
            dbg.contains("<unset>"),
            "expected <unset> token marker: {dbg}"
        );
    }
}
