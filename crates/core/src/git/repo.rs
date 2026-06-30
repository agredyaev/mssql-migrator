use std::path::{Path, PathBuf};

/// Walks up from `start` and returns the first directory that contains a `.git` entry.
pub fn find_repo_root(start: &str) -> Option<PathBuf> {
    let mut dir = Path::new(start).canonicalize().ok()?;
    loop {
        if dir.join(".git").exists() {
            return Some(dir);
        }
        if !dir.pop() {
            break;
        }
    }
    None
}

/// Returns true when `start` is inside a git repository.
pub fn has_git_repo(start: &str) -> bool {
    find_repo_root(start).is_some()
}

/// Returns the slash-terminated path of `sql_root` relative to `repo_root`.
pub fn sql_prefix_in_repo(repo_root: &Path, sql_root: &str) -> Option<String> {
    let sql = Path::new(sql_root).canonicalize().ok()?;
    let rel = sql.strip_prefix(repo_root).ok()?;
    if rel.as_os_str().is_empty() {
        return None;
    }
    let mut s = rel.to_string_lossy().replace('\\', "/");
    if !s.ends_with('/') {
        s.push('/');
    }
    Some(s)
}

/// Returns the slash-terminated SQL directory prefix relative to the repository root.
pub fn sql_path_prefix(sql_root: &str) -> Option<String> {
    let repo = find_repo_root(sql_root)?;
    sql_prefix_in_repo(&repo, sql_root)
}
