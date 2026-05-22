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
/// sequences (`.`, `..`), or includes path separators (`/`, `\`, `\0`).
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
        if matches!(c, '/' | '\\' | '\0') {
            return Err(Error::InvalidInput(format!(
                "invalid character {c:?} in path component: {name:?}"
            )));
        }
    }
    Ok(())
}

fn validate_bracket_name(name: &str) -> Result<()> {
    validate_path_component(name)?;
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
    validate_bracket_name(name)?;
    Ok(format!("[{}]", name.replace(']', "]]")))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn quote_escapes_bracket() {
        assert_eq!(bracket_ident("a]b").unwrap(), "[a]]b]");
    }

    #[test]
    fn rejects_dotdot() {
        assert!(validate_path_component("..").is_err());
    }
}
