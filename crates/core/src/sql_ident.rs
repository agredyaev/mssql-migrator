//! T-SQL identifier sanitization and path component validation.
//!
//! ### Purpose
//! Prevents SQL injection and directory traversal vulnerabilities when handling
//! dynamically resolved schema names, table names, and migration SQL scripts.
//!
//! ### Threats Mitigated
//! 1. **Path Traversal (`../`)**: Scrubbing filenames prevents malicious catalog layout scans
//!    from crossing directory boundaries or reading arbitrary host system files.
//! 2. **T-SQL Bracket Escapes**: Standard T-SQL escapes bracketed identifiers `[name]` by
//!    doubling the closing bracket (`] -> ]]`). Failing to do so allows SQL injection via
//!    malicious schema/table names during dynamic query generation.

use crate::error::{Error, Result};

/// Asserts that a string is a safe single-level directory or filename component.
///
/// Returns `Error::InvalidInput` if the component is empty, contains directory traversal
/// sequences (`.`, `..`), path separators (`/`, `\`), or control characters.
pub fn validate_path_component(name: &str) -> Result<()> {
    if name.is_empty() {
        return Err(Error::InvalidInput("empty path component".into()));
    }
    if name == "." || name == ".." {
        return Err(Error::InvalidInput(format!(
            "invalid path component: {name:?}"
        )));
    }
    for c in name.chars() {
        if matches!(c, '/' | '\\') || c.is_control() {
            return Err(Error::InvalidInput(format!(
                "invalid character {c:?} in path component: {name:?}"
            )));
        }
    }
    Ok(())
}

/// Asserts that a token contains only safe alphanumeric, underscore, or dash characters.
///
/// Commonly used to validate dynamic git SHAs embedded inside migration SQL filenames.
pub fn validate_filename_token(token: &str) -> Result<()> {
    if token.is_empty() {
        return Err(Error::InvalidInput("empty filename token".into()));
    }
    if !token
        .chars()
        .all(|c| c.is_ascii_alphanumeric() || matches!(c, '_' | '-'))
    {
        return Err(Error::InvalidInput(format!(
            "invalid filename token: {token:?}"
        )));
    }
    Ok(())
}

/// Escapes and wraps an identifier in standard T-SQL bracket formatting (e.g. `[object]`).
///
/// Any embedded closing brackets `]` are escaped by doubling them (`]]`) to prevent
/// SQL injection through malicious table or schema names.
pub fn bracket_ident(name: &str) -> Result<String> {
    validate_path_component(name)?;
    // MSSQL regular and quoted identifiers are limited to 128 characters.
    let len = name.chars().count();
    if len > 128 {
        return Err(Error::InvalidInput(format!(
            "identifier too long ({len} chars, max 128): {name:?}"
        )));
    }
    Ok(format!("[{}]", name.replace(']', "]]")))
}

#[cfg(test)]
#[path = "tests/sql_ident_test.rs"]
mod sql_ident_tests;
