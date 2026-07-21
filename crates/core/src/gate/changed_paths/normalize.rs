use super::super::git_diff;
use super::ChangedPathsResult;

pub(super) fn git_paths_result(
    sql_root: &str,
    paths: Vec<String>,
    source: &'static str,
) -> ChangedPathsResult {
    ChangedPathsResult {
        paths: normalize_git_paths(sql_root, paths),
        full_inspect: false,
        source,
    }
}

pub(super) fn normalize_git_paths(sql_root: &str, paths: Vec<String>) -> Vec<String> {
    let prefix = git_diff::find_repo_root(sql_root)
        .and_then(|root| git_diff::sql_prefix(&root, sql_root))
        .unwrap_or_else(|| "sql/".into());
    paths
        .into_iter()
        .filter_map(|raw| {
            let mut p = raw.trim().replace('\\', "/");
            if let Some(stripped) = p.strip_prefix(&prefix) {
                p = stripped.to_string();
            } else if let Some(stripped) = p.strip_prefix("sql/") {
                p = stripped.to_string();
            }
            if p.is_empty() {
                None
            } else {
                Some(p)
            }
        })
        .collect()
}

pub(super) fn full(source: &'static str) -> ChangedPathsResult {
    ChangedPathsResult {
        paths: Vec::new(),
        full_inspect: true,
        source,
    }
}
