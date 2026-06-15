/// Whether `RM_DB_AUTH` selects integrated / Windows SSO instead of SQL login.
pub fn uses_integrated_auth(db_auth: &str) -> bool {
    matches!(
        db_auth.to_ascii_lowercase().as_str(),
        "integrated" | "windows"
    )
}

/// Whether SQL username/password are required for the configured auth mode.
pub fn sql_credentials_required(db_auth: &str) -> bool {
    !uses_integrated_auth(db_auth)
}
