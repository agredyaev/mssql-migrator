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
    // Always build: an existence-only check can run a stale binary against
    // current sources; cargo no-ops when the build is fresh.
    let status = Command::new("cargo")
        .args(["build", "--release", "-p", "rmigd"])
        .current_dir(&root)
        .status()
        .expect("cargo build rmigd");
    assert!(status.success(), "rmigd build failed");
    let _ = std::fs::remove_file(&socket);
    let mut cmd = Command::new(&bin);
    cmd.env("RMIGD_SOCKET", &socket)
        .env("RMIG_SESSION_TOKEN", INTEGRATION_TOKEN);
    let env_path = root.join(".env");
    if env_path.is_file() {
        cmd.env("RMIGD_ENV", &env_path);
    }
    register_exit_cleanup();
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
/// Kills ONLY processes whose command name is `rmigd`: a mistyped or reused
/// RMIGD_SOCKET must never terminate an unrelated socket owner.
fn kill_stale_socket_holder(socket: &str) {
    #[cfg(unix)]
    {
        if let Ok(out) = Command::new("lsof").args(["-t", socket]).output() {
            for line in std::str::from_utf8(&out.stdout).unwrap_or("").lines() {
                let pid = line.trim();
                if pid.is_empty() {
                    continue;
                }
                if !is_rmigd_process(pid) {
                    panic!(
                        "socket {socket} is held by non-rmigd pid {pid}; \
                         refusing to kill it — check RMIGD_SOCKET"
                    );
                }
                let _ = Command::new("kill").arg(pid).status();
            }
        }
    }
    let _ = std::fs::remove_file(socket);
}

#[cfg(unix)]
fn is_rmigd_process(pid: &str) -> bool {
    Command::new("ps")
        .args(["-o", "comm=", "-p", pid])
        .output()
        .ok()
        .map(|o| String::from_utf8_lossy(&o.stdout).contains("rmigd"))
        .unwrap_or(false)
}

/// Kill + reap the spawned daemon when the test process exits: statics are
/// never dropped, so without this every run leaks an rmigd with a live SQL
/// session. Registered once, on first spawn.
fn register_exit_cleanup() {
    use std::sync::Once;
    static ONCE: Once = Once::new();
    extern "C" fn cleanup() {
        if let Ok(mut guard) = CHILD.lock() {
            if let Some(mut child) = guard.take() {
                let _ = child.kill();
                let _ = child.wait();
            }
        }
    }
    ONCE.call_once(|| unsafe {
        extern "C" {
            fn atexit(cb: extern "C" fn()) -> std::os::raw::c_int;
        }
        let _ = atexit(cleanup);
    });
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
