use std::time::Duration;

/// Connection / session / cache paths (**COLD**). Cheap `Arc` clone with [`super::Config`].
#[derive(Clone, Debug, Default)]
pub struct ConfigCold {
    pub server: String,
    pub port: String,
    pub db_auth: String,
    pub user: String,
    pub password: String,
    pub encrypt: bool,
    pub trust_server_certificate: bool,
    pub command_timeout: Duration,
    pub lock_timeout: Duration,
    pub session_socket: String,
    pub session_token: String,
    pub l1_cache_dir: String,
}
