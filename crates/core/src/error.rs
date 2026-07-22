//! System error categorizations, database exception translation, and shell exit code mappings.
//!
//! ### Purpose
//! Converts low-level errors (I/O, TDS database driver exceptions) into structured `Error` states
//! and translates them to standard process exit codes to allow correct automated shell recovery.
//!
//! ### Architectural Context
//! - **Inputs**: Low-level thread errors, database query exceptions, parse failures.
//! - **Outputs**: Formatted error strings, process exit codes (e.g. `10` on blocked migration, `7` on lock held).
//! - **Boundaries**: Used across all crates to ensure standard, consistent failure diagnostics.
//!
//! ### Mapped Exit Codes
//! - `0`: Success (`EXIT_OK`)
//! - `1`: Uncategorized / OS errors (`EXIT_GENERAL`)
//! - `2`: Environmental config errors (`EXIT_CONFIG`)
//! - `3`: Database connection failures (`EXIT_CONN`)
//! - `4`: Undecodable persisted audit checksum; run `repair-checksum` (`EXIT_CHECKSUM`)
//! - `5`: SQL execution runtime exceptions (`EXIT_SQL`)
//! - `7`: Advisory lock timeout / contention (`EXIT_LOCK_TIMEOUT`)
//! - `8`: Invalid input / bad repository structure or identifier (`EXIT_INVALID_INPUT`)
//! - `10`: Structural layout plan is blocked (`EXIT_PLAN_BLOCKED`)

use std::fmt;

/// Top-level error type for all crate operations.
#[derive(Debug)]
pub enum Error {
    /// Invalid or missing configuration value.
    Config(String),
    /// Malformed input, bad repository structure, or invalid identifier.
    InvalidInput(String),
    /// Underlying I/O failure.
    Io(std::io::Error),
    /// SQL execution or driver exception.
    Sql(String),
    /// Connection-phase failure (TCP dial, TDS handshake) — a transport/auth
    /// problem distinct from a SQL execution error, mapped to `EXIT_CONN`.
    Conn(String),
    /// Migration plan is structurally blocked and cannot proceed.
    PlanBlocked,
    /// Persisted audit checksum is malformed and cannot be trusted.
    Checksum(String),
    /// Advisory lock could not be acquired within the timeout.
    LockTimeout,
    /// Uncategorized error from a dependency.
    Other(Box<dyn std::error::Error + Send + Sync>),
}

/// Convenience alias for `std::result::Result<T, Error>`.
pub type Result<T> = std::result::Result<T, Error>;

/// Exit code returned on success (0).
pub const EXIT_OK: i32 = 0;
/// Exit code returned on uncategorized or I/O errors (1).
pub const EXIT_GENERAL: i32 = 1;
/// Exit code returned on configuration errors (2).
pub const EXIT_CONFIG: i32 = 2;
/// Exit code returned on database connection failure (3).
pub const EXIT_CONN: i32 = 3;
/// Exit code reserved for checksum failures (4).
pub const EXIT_CHECKSUM: i32 = 4;
/// Exit code returned on SQL execution errors (5).
pub const EXIT_SQL: i32 = 5;
/// Exit code returned when the advisory lock times out (7).
pub const EXIT_LOCK_TIMEOUT: i32 = 7;
/// Exit code returned on invalid input or bad repository structure (8).
pub const EXIT_INVALID_INPUT: i32 = 8;
/// Exit code returned when the migration plan is blocked (10).
pub const EXIT_PLAN_BLOCKED: i32 = 10;

impl Error {
    /// Returns the process exit code that corresponds to this error variant.
    pub fn exit_code(&self) -> i32 {
        match self {
            Self::Config(_) => EXIT_CONFIG,
            Self::InvalidInput(_) => EXIT_INVALID_INPUT,
            Self::PlanBlocked => EXIT_PLAN_BLOCKED,
            Self::Checksum(_) => EXIT_CHECKSUM,
            Self::LockTimeout => EXIT_LOCK_TIMEOUT,
            Self::Conn(_) => EXIT_CONN,
            Self::Sql(_) => EXIT_SQL,
            Self::Io(_) => EXIT_GENERAL,
            Self::Other(_) => EXIT_GENERAL,
        }
    }
}

impl fmt::Display for Error {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Config(m) => write!(f, "configuration error: {m}"),
            Self::InvalidInput(m) => write!(f, "{m}"),
            Self::Io(e) => write!(f, "{e}"),
            Self::Sql(m) => write!(f, "{m}"),
            Self::Conn(m) => write!(f, "{m}"),
            Self::PlanBlocked => write!(f, "plan is blocked"),
            Self::Checksum(m) => write!(f, "checksum integrity error: {m}"),
            Self::LockTimeout => write!(f, "lock timeout"),
            Self::Other(e) => write!(f, "{e}"),
        }
    }
}

impl std::error::Error for Error {}

impl From<std::io::Error> for Error {
    fn from(e: std::io::Error) -> Self {
        Self::Io(e)
    }
}

impl From<Box<dyn std::error::Error + Send + Sync>> for Error {
    fn from(e: Box<dyn std::error::Error + Send + Sync>) -> Self {
        Self::Other(e)
    }
}

#[cfg(test)]
#[path = "tests/error_test.rs"]
mod error_tests;
