mod changed_env;
mod normalize;

use std::env;

use super::changed_paths_ci;
use super::git_diff;

/// Outcome of changed-paths resolution, including the paths list and the source that produced it.
#[derive(Debug, Clone)]
pub struct ChangedPathsResult {
    /// SQL-root-relative paths of files that changed.
    pub paths: Vec<String>,
    /// Whether the resolver fell back to a full inspection (all files).
    pub full_inspect: bool,
    /// Label identifying which resolution strategy was used.
    pub source: &'static str,
}

/// Resolves the set of changed SQL paths using environment variables, git, or CI metadata.
pub fn resolve_changed_paths(sql_root: &str) -> ChangedPathsResult {
    if env::var("RMIG_INSPECT_FULL").as_deref() == Ok("1") {
        return normalize::full("env-full-inspect");
    }
    if let Some(paths) = changed_env::changed_paths_from_env() {
        return normalize::git_paths_result(sql_root, paths, "env-changed-files");
    }
    if let Ok(base) = env::var("RMIG_GATE_GIT_BASE") {
        let base = base.trim();
        if !base.is_empty() {
            return match git_diff::find_repo_root(sql_root) {
                Some(root) => match git_diff::changed_paths_from_git(&root, base) {
                    Some(paths) => normalize::git_paths_result(sql_root, paths, "env-git-base"),
                    None => normalize::fail("env-git-base-failed"),
                },
                None => normalize::fail("env-git-base-no-git"),
            };
        }
    }
    let Some(repo_root) = git_diff::find_repo_root(sql_root) else {
        return normalize::full("no-git");
    };
    if let Some((paths, source)) = changed_paths_ci::try_ci_changed_paths(&repo_root) {
        return normalize::git_paths_result(sql_root, paths, source);
    }
    if let Some(paths) = git_diff::merge_base_paths(sql_root) {
        if !paths.is_empty() {
            return normalize::git_paths_result(sql_root, paths, "git-merge-base");
        }
    }
    match git_diff::changed_paths_from_git(&repo_root, "HEAD") {
        Some(paths) if !paths.is_empty() => {
            normalize::git_paths_result(sql_root, paths, "git-head")
        }
        _ => {
            if let Some(paths) = git_diff::changed_paths_from_git(&repo_root, "HEAD~1") {
                if !paths.is_empty() {
                    return normalize::git_paths_result(sql_root, paths, "git-head-parent");
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

#[cfg(test)]
mod tests {
    use super::normalize::normalize_git_paths;
    use std::env;

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
