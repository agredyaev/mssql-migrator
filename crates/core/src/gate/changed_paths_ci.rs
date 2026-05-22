use std::env;
use std::fs;
use std::process::Command;

use super::git_diff;

pub fn try_ci_changed_paths(repo_root: &str) -> Option<(Vec<String>, &'static str)> {
    if let Some(paths) = github_pr_paths(repo_root) {
        return Some((paths, "ci-github-pr"));
    }
    if let Some(paths) = gitlab_mr_paths(repo_root) {
        return Some((paths, "ci-gitlab-mr"));
    }
    if let Some(paths) = azure_pr_paths(repo_root) {
        return Some((paths, "ci-ado-pr"));
    }
    None
}

fn github_pr_paths(repo_root: &str) -> Option<Vec<String>> {
    if env::var("GITHUB_EVENT_NAME").ok().as_deref() != Some("pull_request") {
        return None;
    }
    let path = env::var("GITHUB_EVENT_PATH").ok()?;
    let data = fs::read_to_string(path).ok()?;
    let payload: serde_json::Value = serde_json::from_str(&data).ok()?;
    let base = payload
        .pointer("/pull_request/base/sha")
        .and_then(|v| v.as_str())
        .map(str::trim)
        .filter(|s| !s.is_empty())?;
    git_diff::diff_name_only(repo_root, base, "HEAD")
}

fn gitlab_mr_paths(repo_root: &str) -> Option<Vec<String>> {
    env::var("CI_MERGE_REQUEST_IID")
        .ok()
        .filter(|s| !s.is_empty())?;
    let base = env::var("CI_MERGE_REQUEST_DIFF_BASE_SHA").ok()?;
    let head = env::var("CI_COMMIT_SHA").ok()?;
    if base.trim().is_empty() || head.trim().is_empty() {
        return None;
    }
    git_diff::diff_name_only(repo_root, base.trim(), head.trim())
}

fn azure_pr_paths(repo_root: &str) -> Option<Vec<String>> {
    let target = env::var("SYSTEM_PULLREQUEST_TARGETBRANCH")
        .ok()
        .map(|s| s.trim().to_string())
        .filter(|s| !s.is_empty());
    if target.is_none() && env::var("BUILD_REASON").ok().as_deref() != Some("PullRequest") {
        return None;
    }
    let target = target?;
    let target = target.strip_prefix("refs/heads/").unwrap_or(&target);
    let remote = format!("origin/{target}");
    let _ = Command::new("git")
        .args([
            "-C",
            repo_root,
            "fetch",
            "origin",
            &format!("{target}:{target}"),
            "--depth=1",
        ])
        .status();
    if let Some(base) = git_diff::merge_base(repo_root, "HEAD", &remote) {
        return git_diff::diff_name_only(repo_root, &base, "HEAD");
    }
    git_diff::changed_paths_from_git(repo_root, &remote)
}
