use std::process::Command;

use super::repo;

pub fn diff_name_only(repo: &str, base: &str, head: &str) -> Option<Vec<String>> {
    let out = Command::new("git")
        .args(["-C", repo, "diff", "--name-only", base, head])
        .output()
        .ok()?;
    if !out.status.success() {
        return None;
    }
    Some(parse_name_only(&String::from_utf8_lossy(&out.stdout)))
}

pub fn changed_paths_from_git(repo: &str, base_ref: &str) -> Option<Vec<String>> {
    diff_name_only(repo, base_ref, "HEAD")
}

pub fn merge_base_paths(sql_root: &str) -> Option<Vec<String>> {
    let root = repo::find_repo_root(sql_root)?
        .to_string_lossy()
        .into_owned();
    for remote in ["origin/main", "origin/master", "main", "master"] {
        if let Some(paths) = diff_since_merge_base(&root, remote) {
            return Some(paths);
        }
    }
    None
}

pub fn merge_base(repo: &str, head: &str, other: &str) -> Option<String> {
    let out = Command::new("git")
        .args(["-C", repo, "merge-base", head, other])
        .output()
        .ok()?;
    if !out.status.success() {
        return None;
    }
    let s = String::from_utf8_lossy(&out.stdout).trim().to_string();
    if s.is_empty() {
        None
    } else {
        Some(s)
    }
}

fn diff_since_merge_base(repo: &str, remote: &str) -> Option<Vec<String>> {
    let base = merge_base(repo, "HEAD", remote)?;
    diff_name_only(repo, &base, "HEAD")
}

pub fn parse_name_only(stdout: &str) -> Vec<String> {
    stdout
        .lines()
        .map(|l| l.trim().replace('\\', "/"))
        .filter(|l| !l.is_empty())
        .collect()
}
