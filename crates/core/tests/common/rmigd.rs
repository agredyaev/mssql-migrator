//! Start release `rmigd` for integration/SLO when `RMIG_USE_RMIGD=1`.

use std::path::PathBuf;
use std::process::{Child, Command, Stdio};
use std::sync::Mutex;
use std::time::Duration;

static CHILD: Mutex<Option<Child>> = Mutex::new(None);

const INTEGRATION_TOKEN: &str = "rmig-integration-test-token";

pub fn ensure_started() -> Option<String> {
    if !use_rmigd() {
        return None;
    }
    std::env::set_var("RMIG_SESSION_TOKEN", INTEGRATION_TOKEN);
    let socket = socket_path();
    {
        let mut guard = CHILD.lock().expect("rmigd child lock");
        if let Some(child) = guard.as_mut() {
            if child.try_wait().ok().flatten().is_none() {
                return Some(socket);
            }
            *guard = None;
        }
    }
    kill_stale_socket_holder(&socket);
    let root = super::repo_root();
    if let Some(parent) = PathBuf::from(&socket).parent() {
        let _ = std::fs::create_dir_all(parent);
    }
    let bin = root.join("target/release/rmigd");
    if !bin.exists() {
        let status = Command::new("cargo")
            .args(["build", "--release", "-p", "rmigd"])
            .current_dir(&root)
            .status()
            .expect("cargo build rmigd");
        assert!(status.success(), "rmigd build failed");
    }
    let _ = std::fs::remove_file(&socket);
    let mut cmd = Command::new(&bin);
    cmd.env("RMIGD_SOCKET", &socket)
        .env("RMIG_SESSION_TOKEN", INTEGRATION_TOKEN);
    let env_path = root.join(".env");
    if env_path.is_file() {
        cmd.env("RMIGD_ENV", &env_path);
    }
    let child = cmd
        .env(
            "RM_DB_SERVER",
            std::env::var("RM_DB_SERVER").unwrap_or_else(|_| "127.0.0.1".into()),
        )
        .env(
            "RM_DB_PORT",
            std::env::var("RM_DB_PORT").unwrap_or_else(|_| "1433".into()),
        )
        .env(
            "RM_DB_USER",
            std::env::var("RM_DB_USER").unwrap_or_else(|_| "sa".into()),
        )
        .env(
            "RM_DB_PASSWORD",
            std::env::var("RM_DB_PASSWORD").unwrap_or_else(|_| "yourStrong(!)Password".into()),
        )
        .env("RM_DB_TRUST_SERVER_CERTIFICATE", "true")
        .env(
            "RM_SQL_ROOT",
            std::env::var("RM_SQL_ROOT")
                .unwrap_or_else(|_| root.join(".temp/sql").to_string_lossy().into_owned()),
        )
        .env(
            "RM_SQL_BASE",
            std::env::var("RM_SQL_BASE")
                .unwrap_or_else(|_| root.join(".temp/sql").to_string_lossy().into_owned()),
        )
        .stdout(Stdio::null())
        .stderr(Stdio::inherit())
        .spawn()
        .expect("spawn rmigd");
    wait_socket(&socket);
    *CHILD.lock().expect("rmigd child lock") = Some(child);
    Some(socket)
}

/// Drop orphaned daemons left by prior test runs so advisory locks are not held.
fn kill_stale_socket_holder(socket: &str) {
    #[cfg(unix)]
    {
        if let Ok(out) = Command::new("lsof").args(["-t", socket]).output() {
            for line in std::str::from_utf8(&out.stdout).unwrap_or("").lines() {
                let pid = line.trim();
                if !pid.is_empty() {
                    let _ = Command::new("kill").arg(pid).status();
                }
            }
        }
    }
    let _ = std::fs::remove_file(socket);
}

fn use_rmigd() -> bool {
    matches!(
        std::env::var("RMIG_USE_RMIGD").as_deref(),
        Ok("1") | Ok("true") | Ok("yes")
    )
}

fn socket_path() -> String {
    std::env::var("RMIGD_SOCKET").unwrap_or_else(|_| {
        let default_path = super::repo_root()
            .join(".rmig/rmigd-integration.sock")
            .to_string_lossy()
            .into_owned();
        if default_path.len() >= 100 && cfg!(unix) {
            use std::hash::{Hash, Hasher};
            let mut hasher = std::collections::hash_map::DefaultHasher::new();
            super::repo_root().hash(&mut hasher);
            let hash = hasher.finish();
            format!("/tmp/rmigd-{:x}.sock", hash)
        } else {
            default_path
        }
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
