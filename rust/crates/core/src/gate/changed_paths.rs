use std::env;

use super::changed_paths_ci;
use super::git_diff;

#[derive(Debug, Clone)]
pub struct ChangedPathsResult {
    pub paths: Vec<String>,
    pub full_inspect: bool,
    pub source: &'static str,
}

pub fn resolve_changed_paths(sql_root: &str) -> ChangedPathsResult {
    if env::var("RMIG_INSPECT_FULL").as_deref() == Ok("1") {
        return full("env-full-inspect");
    }
    if let Some(paths) = changed_paths_from_env() {
        return git_paths_result(sql_root, paths, "env-changed-files");
    }
    if let Ok(base) = env::var("RMIG_GATE_GIT_BASE") {
        let base = base.trim();
        if !base.is_empty() {
            return match git_diff::find_repo_root(sql_root) {
                Some(root) => match git_diff::changed_paths_from_git(&root, base) {
                    Some(paths) => git_paths_result(sql_root, paths, "env-git-base"),
                    None => fail("env-git-base-failed"),
                },
                None => fail("env-git-base-no-git"),
            };
        }
    }
    let Some(repo_root) = git_diff::find_repo_root(sql_root) else {
        return full("no-git");
    };
    if let Some((paths, source)) = changed_paths_ci::try_ci_changed_paths(&repo_root) {
        return git_paths_result(sql_root, paths, source);
    }
    if let Some(paths) = git_diff::merge_base_paths(sql_root) {
        if !paths.is_empty() {
            return git_paths_result(sql_root, paths, "git-merge-base");
        }
    }
    match git_diff::changed_paths_from_git(&repo_root, "HEAD") {
        Some(paths) if !paths.is_empty() => git_paths_result(sql_root, paths, "git-head"),
        _ => {
            if let Some(paths) = git_diff::changed_paths_from_git(&repo_root, "HEAD~1") {
                if !paths.is_empty() {
                    return git_paths_result(sql_root, paths, "git-head-parent");
                }
            }
            ChangedPathsResult {
                paths: Vec::new(),
                full_inspect: false,
                source: "git-head",
            }
        }
    }
}

fn git_paths_result(
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

fn normalize_git_paths(sql_root: &str, paths: Vec<String>) -> Vec<String> {
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

fn full(source: &'static str) -> ChangedPathsResult {
    ChangedPathsResult {
        paths: Vec::new(),
        full_inspect: true,
        source,
    }
}

fn fail(source: &'static str) -> ChangedPathsResult {
    ChangedPathsResult {
        paths: Vec::new(),
        full_inspect: true,
        source,
    }
}

fn changed_paths_from_env() -> Option<Vec<String>> {
    env::var("RMIG_GATE_CHANGED_FILES")
        .ok()
        .and_then(|raw| parse_csv_paths(&raw))
        .or_else(|| {
            env::var("RMIG_CHANGED_FILES")
                .ok()
                .and_then(|raw| parse_csv_paths(&raw))
        })
}

fn parse_csv_paths(raw: &str) -> Option<Vec<String>> {
    let paths: Vec<_> = raw
        .split(',')
        .map(|s| s.trim().replace('\\', "/"))
        .filter(|s| !s.is_empty())
        .collect();
    if paths.is_empty() {
        None
    } else {
        Some(paths)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn normalize_strips_sql_prefix() {
        let root = env!("CARGO_MANIFEST_DIR");
        let sql_root = format!("{root}/../../../.temp/sql");
        let paths = normalize_git_paths(
            &sql_root,
            vec!["sql/dactests/smoke/tables/smoke_table.sql".into()],
        );
        assert_eq!(paths, vec!["dactests/smoke/tables/smoke_table.sql"]);
    }
}
