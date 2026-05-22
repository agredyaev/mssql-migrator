use std::path::Path;

pub fn has_git_repo(start: &str) -> bool {
    crate::git::has_git_repo(start)
}

pub fn sql_path_prefix(sql_root: &str) -> Option<String> {
    crate::git::sql_path_prefix(sql_root)
}

pub fn git_work_tree(sql_root: &str) -> Option<String> {
    Path::new(sql_root)
        .canonicalize()
        .ok()
        .map(|p| p.to_string_lossy().into_owned())
}
