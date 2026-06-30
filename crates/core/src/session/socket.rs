//! Default rmigd socket path and directory permissions.
//!
//! ### Non-obvious
//! - **`restrict_socket_mode`** and **`restrict_dir_mode`** are no-ops on
//!   non-Unix platforms (`#[cfg(not(unix))]`). On Unix, socket is `0o600`,
//!   directory `0o700`.
//! - Socket path defaults to `~/.rmig/rmigd.sock`, overridable via `RMIGD_SOCKET`.
//! - Directory parents are created on resolve if they do not exist.

use std::path::{Path, PathBuf};

use crate::error::{Error, Result};

/// `~/.rmig/rmigd.sock` (or `$HOME/.rmig/rmigd.sock`).
pub fn default_socket_path() -> PathBuf {
    rmig_dir().join("rmigd.sock")
}

/// Returns the path to the `~/.rmig` directory.
pub fn rmig_dir() -> PathBuf {
    if let Ok(home) = std::env::var("HOME") {
        PathBuf::from(home).join(".rmig")
    } else {
        PathBuf::from(".rmig")
    }
}

/// Create `~/.rmig` with mode `0700` and return the path.
pub fn ensure_rmig_dir() -> Result<PathBuf> {
    let dir = rmig_dir();
    std::fs::create_dir_all(&dir).map_err(Error::Io)?;
    restrict_dir_mode(&dir)?;
    Ok(dir)
}

/// Resolve socket path: `RMIGD_SOCKET` env, else [`default_socket_path`].
pub fn resolve_socket_path() -> Result<PathBuf> {
    let path = std::env::var("RMIGD_SOCKET")
        .map(PathBuf::from)
        .unwrap_or_else(|_| default_socket_path());
    if let Some(parent) = path.parent() {
        if parent != Path::new("") {
            std::fs::create_dir_all(parent).map_err(Error::Io)?;
            restrict_dir_mode(parent)?;
        }
    }
    Ok(path)
}

/// Restrict socket to owner-only (`0o600`). No-op on non-Unix.
#[cfg(unix)]
pub fn restrict_socket_mode(path: &Path) -> Result<()> {
    use std::os::unix::fs::PermissionsExt;
    std::fs::set_permissions(path, std::fs::Permissions::from_mode(0o600)).map_err(Error::Io)
}

/// No-op on non-Unix — Windows does not support Unix permission bits.
#[cfg(not(unix))]
pub fn restrict_socket_mode(_path: &Path) -> Result<()> {
    Ok(())
}

/// Restrict directory to owner-only (`0o700`). No-op on non-Unix.
#[cfg(unix)]
fn restrict_dir_mode(path: &Path) -> Result<()> {
    use std::os::unix::fs::PermissionsExt;
    std::fs::set_permissions(path, std::fs::Permissions::from_mode(0o700)).map_err(Error::Io)
}

/// No-op on non-Unix.
#[cfg(not(unix))]
fn restrict_dir_mode(_path: &Path) -> Result<()> {
    Ok(())
}
