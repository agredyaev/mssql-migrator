//! Start release `rmigd` for integration/SLO when `RMIG_USE_RMIGD=1`.

use std::path::PathBuf;
use std::process::{Child, Command, Stdio};
use std::sync::Mutex;
use std::time::Duration;

static CHILD: Mutex<Option<(Child, String)>> = Mutex::new(None);

const INTEGRATION_TOKEN: &str = "rmig-integration-test-token";

pub fn ensure_started() -> Option<String> {
    if !use_rmigd() {
        return None;
    }
    std::env::set_var("RMIG_SESSION_TOKEN", INTEGRATION_TOKEN);
    let socket = socket_path();
    {
        let mut guard = CHILD.lock().expect("rmigd child lock");
        if let Some((child, owned_socket)) = guard.as_mut() {
            if child.try_wait().ok().flatten().is_none() {
                return Some(socket);
            }
            let _ = std::fs::remove_file(owned_socket);
            *guard = None;
        }
    }
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
    assert_socket_available(&socket);
    let mut cmd = Command::new(&bin);
    cmd.env("RMIGD_SOCKET", &socket)
        .env("RMIG_SESSION_TOKEN", INTEGRATION_TOKEN);
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
        // The checked-in Docker SQL Server fixture has no TLS endpoint.
        // Keep this test-only opt-out explicit; production defaults stay secure.
        .env(
            "RM_DB_ENCRYPT",
            std::env::var("RM_DB_ENCRYPT").unwrap_or_else(|_| "false".into()),
        )
        .env(
            "RM_DB_TRUST_SERVER_CERTIFICATE",
            std::env::var("RM_DB_TRUST_SERVER_CERTIFICATE").unwrap_or_else(|_| "true".into()),
        )
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
    *CHILD.lock().expect("rmigd child lock") = Some((child, socket.clone()));
    Some(socket)
}

/// Refuse every pre-existing override. Process-name checks do not prove that a
/// daemon belongs to this harness, and deleting an arbitrary stale path is not
/// safe either.
fn assert_socket_available(socket: &str) {
    if std::path::Path::new(socket).exists() {
        panic!(
            "rmigd integration socket already exists: {socket}; \
             refusing to stop or remove a process/path not owned by this harness"
        );
    }
}

/// Kill + reap the spawned daemon when the test process exits: statics are
/// never dropped, so without this every run leaks an rmigd with a live SQL
/// session. Registered once, on first spawn.
fn register_exit_cleanup() {
    use std::sync::Once;
    static ONCE: Once = Once::new();
    extern "C" fn cleanup() {
        if let Ok(mut guard) = CHILD.lock() {
            if let Some((child, socket)) = guard.take() {
                let _ = terminate_owned(child, &socket);
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

fn terminate_owned(mut child: Child, socket: &str) -> std::io::Result<std::process::ExitStatus> {
    let _ = child.kill();
    let status = child.wait()?;
    match std::fs::remove_file(socket) {
        Ok(()) => {}
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
        Err(e) => return Err(e),
    }
    Ok(status)
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
            .join(format!(
                ".rmig/rmigd-integration-{}.sock",
                std::process::id()
            ))
            .to_string_lossy()
            .into_owned();
        if default_path.len() >= 100 && cfg!(unix) {
            use std::hash::{Hash, Hasher};
            let mut hasher = std::collections::hash_map::DefaultHasher::new();
            super::repo_root().hash(&mut hasher);
            let hash = hasher.finish();
            format!("/tmp/rmigd-{:x}-{}.sock", hash, std::process::id())
        } else {
            default_path
        }
    })
}

#[test]
fn preexisting_socket_override_is_never_removed_regression() {
    let path = std::env::temp_dir().join(format!(
        "rmigd-unowned-{}-{}.sock",
        std::process::id(),
        std::thread::current().name().unwrap_or("test")
    ));
    std::fs::write(&path, b"not owned by harness").expect("create sentinel");
    let result = std::panic::catch_unwind(|| {
        assert_socket_available(path.to_str().expect("utf8 path"));
    });
    assert!(result.is_err(), "pre-existing path must be rejected");
    assert_eq!(
        std::fs::read(&path).expect("sentinel must remain"),
        b"not owned by harness"
    );
    std::fs::remove_file(path).expect("cleanup sentinel");
}

#[cfg(unix)]
#[test]
fn owned_child_is_reaped_and_socket_removed_regression() {
    let path = std::env::temp_dir().join(format!(
        "rmigd-owned-{}-{}.sock",
        std::process::id(),
        std::thread::current().name().unwrap_or("test")
    ));
    std::fs::write(&path, b"owned by harness").expect("create owned sentinel");
    let child = Command::new("sleep")
        .arg("60")
        .spawn()
        .expect("spawn owned child");
    let status = terminate_owned(child, path.to_str().expect("utf8 path"))
        .expect("kill, reap, and unlink owned resources");
    assert!(!status.success(), "killed child must not exit successfully");
    assert!(!path.exists(), "owned socket must be removed");
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
