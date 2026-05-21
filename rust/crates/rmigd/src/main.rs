use std::path::PathBuf;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let socket = migrator_core::session::resolve_socket_path()?;
    let env = std::env::var("RMIGD_ENV").unwrap_or_else(|_| ".env".into());
    migrator_core::session::run_daemon(&socket, PathBuf::from(env).as_path()).await
}
