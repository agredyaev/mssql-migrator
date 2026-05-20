//! T-SQL bracket identifier quoting and safe path component validation.

use crate::error::{Error, Result};

/// Reject filesystem path components (`..`, separators).
pub fn validate_path_component(name: &str) -> Result<()> {
    if name.is_empty() {
        return Err(Error::InvalidInput("empty path component".into()));
    }
    if name == "." || name == ".." {
        return Err(Error::InvalidInput(format!("invalid path component: {name:?}")));
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

/// Safe filename token (e.g. git short hash in scaffold SQL names).
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

/// Full T-SQL bracket identifier `[name]` with `]` escaped as `]]`.
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
