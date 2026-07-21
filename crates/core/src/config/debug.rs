use std::fmt;

use super::Config;

impl fmt::Debug for Config {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.debug_struct("Config")
            .field("sql_root", &self.sql_root)
            .field("sql_base", &self.sql_base)
            .field("report_dir", &self.report_dir)
            .field("server", &self.server)
            .field("port", &self.port)
            .field("database", &self.database)
            .field("user", &self.user)
            .field("password", &"<redacted>")
            .field("session_socket", &self.session_socket)
            .field("session_token", &mask_token(&self.session_token))
            .field("encrypt", &self.encrypt)
            .field("trust_server_certificate", &self.trust_server_certificate)
            .field("report_sync", &self.report_sync)
            .field("skip_git", &self.skip_git)
            .field("catalog_cache", &self.catalog_cache)
            .finish_non_exhaustive()
    }
}

fn mask_token(token: &str) -> &'static str {
    if token.is_empty() {
        "<unset>"
    } else {
        "<redacted>"
    }
}
