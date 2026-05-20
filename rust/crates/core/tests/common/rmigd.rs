//! Start release `rmigd` for integration/SLO when `RMIG_USE_RMIGD=1`.

use std::path::PathBuf;
use std::process::{Child, Command, Stdio};
use std::sync::OnceLock;
use std::time::Duration;

static CHILD: OnceLock<Child> = OnceLock::new();

const INTEGRATION_TOKEN: &str = "rmig-integration-test-token";

pub fn ensure_started() -> Option<String> {
    if !use_rmigd() {
        return None;
    }
    std::env::set_var("RMIG_SESSION_TOKEN", INTEGRATION_TOKEN);
    let socket = socket_path();
    if CHILD.get().is_some() {
        return Some(socket);
    }
    let root = super::repo_root();
    if let Some(parent) = PathBuf::from(&socket).parent() {
        let _ = std::fs::create_dir_all(parent);
    }
    let bin = root.join("rust/target/release/rmigd");
    if !bin.exists() {
        let status = Command::new("cargo")
            .args(["build", "--release", "-p", "rmigd"])
            .current_dir(root.join("rust"))
            .status()
            .expect("cargo build rmigd");
        assert!(status.success(), "rmigd build failed");
    }
    let _ = std::fs::remove_file(&socket);
    let child = Command::new(&bin)
        .env("RMIGD_SOCKET", &socket)
        .env("RMIG_SESSION_TOKEN", INTEGRATION_TOKEN)
        .env("RMIGD_ENV", root.join(".env"))
        .env("RM_DB_SERVER", std::env::var("RM_DB_SERVER").unwrap_or_else(|_| "127.0.0.1".into()))
        .env("RM_DB_PORT", std::env::var("RM_DB_PORT").unwrap_or_else(|_| "1433".into()))
        .env("RM_DB_USER", std::env::var("RM_DB_USER").unwrap_or_else(|_| "sa".into()))
        .env(
            "RM_DB_PASSWORD",
            std::env::var("RM_DB_PASSWORD").unwrap_or_else(|_| "yourStrong(!)Password".into()),
        )
        .env("RM_DB_TRUST_SERVER_CERTIFICATE", "true")
        .env("RM_SQL_ROOT", std::env::var("RM_SQL_ROOT").unwrap_or_default())
        .env("RM_SQL_BASE", std::env::var("RM_SQL_BASE").unwrap_or_default())
        .stdout(Stdio::null())
        .stderr(Stdio::inherit())
        .spawn()
        .expect("spawn rmigd");
    wait_socket(&socket);
    CHILD.set(child).ok();
    Some(socket)
}

fn use_rmigd() -> bool {
    matches!(
        std::env::var("RMIG_USE_RMIGD").as_deref(),
        Ok("1") | Ok("true") | Ok("yes")
    )
}

fn socket_path() -> String {
    std::env::var("RMIGD_SOCKET").unwrap_or_else(|_| {
        super::repo_root()
            .join(".rmig/rmigd-integration.sock")
            .to_string_lossy()
            .into_owned()
    })
}

fn wait_socket(path: &str) {
    let p = PathBuf::from(path);
    for _ in 0..50 {
        if p.exists() {
            return;
        }
        std::thread::sleep(Duration::from_millis(100));
    }
    panic!("rmigd socket not ready: {path}");
}
