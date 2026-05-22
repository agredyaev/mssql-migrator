use std::path::{Path, PathBuf};

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

pub fn has_git_repo(start: &str) -> bool {
    find_repo_root(start).is_some()
}

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

pub fn sql_path_prefix(sql_root: &str) -> Option<String> {
    let repo = find_repo_root(sql_root)?;
    sql_prefix_in_repo(&repo, sql_root)
}
