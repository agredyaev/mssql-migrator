/// Whether SQL username/password are required for the configured auth mode.
pub fn sql_credentials_required(_db_auth: &str) -> bool {
    true
}
