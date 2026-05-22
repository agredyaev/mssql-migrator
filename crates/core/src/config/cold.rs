use std::time::Duration;

use super::flags::{
    flag_get, flag_set, CONFIG_COLD_FLAG_ENCRYPT, CONFIG_COLD_FLAG_TRUST_SERVER_CERTIFICATE,
};

/// Connection, session, and cache paths. Cheap `Arc` clone with [`super::Config`].
#[derive(Clone, Debug, Default)]
pub struct ConfigCold {
    pub server: String,
    pub port: String,
    pub db_auth: String,
    pub user: String,
    pub password: String,
    pub session_socket: String,
    pub session_token: String,
    pub l1_cache_dir: String,
    pub command_timeout: Duration,
    pub lock_timeout: Duration,
    pub cold_flags: u8,
}

impl ConfigCold {
    #[inline]
    pub fn encrypt(&self) -> bool {
        flag_get(self.cold_flags, CONFIG_COLD_FLAG_ENCRYPT)
    }

    #[inline]
    pub fn set_encrypt(&mut self, on: bool) {
        flag_set(&mut self.cold_flags, CONFIG_COLD_FLAG_ENCRYPT, on);
    }

    #[inline]
    pub fn trust_server_certificate(&self) -> bool {
        flag_get(self.cold_flags, CONFIG_COLD_FLAG_TRUST_SERVER_CERTIFICATE)
    }

    #[inline]
    pub fn set_trust_server_certificate(&mut self, on: bool) {
        flag_set(
            &mut self.cold_flags,
            CONFIG_COLD_FLAG_TRUST_SERVER_CERTIFICATE,
            on,
        );
    }
}
