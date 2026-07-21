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

/// Resolve socket path: `RMIGD_SOCKET` env, else [`default_socket_path`].
pub fn resolve_socket_path() -> Result<PathBuf> {
    let path = std::env::var("RMIGD_SOCKET")
        .map(PathBuf::from)
        .unwrap_or_else(|_| default_socket_path());
    if let Some(parent) = path.parent() {
        if parent != Path::new("") {
            ensure_private_parent(parent)?;
        }
    }
    Ok(path)
}

/// Create `parent` privately, or require an EXISTING parent to already be
/// private. A pre-existing caller-selected directory (RMIGD_SOCKET) is never
/// chmodded: silently revoking group/world access on, say, `/tmp` or a shared
/// runtime dir would break unrelated software.
pub(crate) fn ensure_private_parent(parent: &Path) -> Result<()> {
    if !parent.exists() {
        std::fs::create_dir_all(parent).map_err(Error::Io)?;
        return restrict_dir_mode(parent);
    }
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let mode = std::fs::metadata(parent)
            .map_err(Error::Io)?
            .permissions()
            .mode();
        if mode & 0o077 != 0 {
            return Err(Error::Config(format!(
                "socket parent {} is group/world accessible (mode {:o}); \
                 point RMIGD_SOCKET at a private directory (0700) or a new path",
                parent.display(),
                mode & 0o777
            )));
        }
    }
    Ok(())
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
pub(crate) fn restrict_dir_mode(path: &Path) -> Result<()> {
    use std::os::unix::fs::PermissionsExt;
    std::fs::set_permissions(path, std::fs::Permissions::from_mode(0o700)).map_err(Error::Io)
}

/// No-op on non-Unix.
#[cfg(not(unix))]
pub(crate) fn restrict_dir_mode(_path: &Path) -> Result<()> {
    Ok(())
}

#[cfg(test)]
#[path = "../tests/socket_parent_test.rs"]
mod socket_parent_tests;
