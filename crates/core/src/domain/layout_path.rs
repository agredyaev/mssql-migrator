/// Catalog-relative SQL path: `{database}/{schema}/...`.
pub fn with_database_prefix(database: &str, path: &str) -> String {
    object_path_for_entry(database, path.replace('\\', "/"))
}

/// Build the catalog-relative path for an object entry.
pub fn object_path_for_entry(database: &str, script_path: String) -> String {
    let path = script_path.as_str();
    if database.is_empty() {
        return script_path;
    }
    let prefix = format!("{database}/");
    if path.starts_with(prefix.as_str()) {
        script_path
    } else {
        format!("{database}/{}", path.trim_start_matches('/'))
    }
}

/// Returns candidate path strings for looking up `path` with and without the database prefix.
pub fn path_lookup_candidates(database: &str, path: &str) -> Vec<String> {
    let mut out = vec![path.to_string()];
    if database.is_empty() {
        return out;
    }
    let prefixed = with_database_prefix(database, path);
    if prefixed != path {
        out.push(prefixed);
    }
    let prefix = format!("{database}/");
    if path.starts_with(prefix.as_str()) {
        out.push(path[prefix.len()..].to_string());
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn leaves_prefixed_path_unchanged() {
        assert_eq!(
            with_database_prefix("dactests", "dactests/smoke/tables/t1.sql"),
            "dactests/smoke/tables/t1.sql"
        );
    }

    #[test]
    fn prepends_database_when_missing() {
        assert_eq!(
            with_database_prefix("dactests", "smoke/tables/t1.sql"),
            "dactests/smoke/tables/t1.sql"
        );
    }
}
