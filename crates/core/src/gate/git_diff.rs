//! Re-exports git diff utilities for use by the gate layer.

pub use crate::git::{
    changed_paths_from_git, diff_name_only, merge_base, merge_base_paths, parse_name_only,
};

/// Returns the repository root path above `start`, or `None` if no git repo is found.
pub fn find_repo_root(start: &str) -> Option<String> {
    crate::git::find_repo_root(start).map(|p| p.to_string_lossy().into_owned())
}

/// Returns the relative SQL prefix inside `repo_root` for the given `sql_root`, or `None`.
pub fn sql_prefix(repo_root: &str, sql_root: &str) -> Option<String> {
    crate::git::sql_prefix_in_repo(std::path::Path::new(repo_root), sql_root)
}
