use std::process::Command;

/// Git commit metadata for a single file.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct GitMeta {
    /// Full commit hash.
    pub hash: String,
    /// Commit author name.
    pub author: String,
    /// Author date in ISO 8601 format.
    pub date: String,
}

// Fields are separated by the ASCII unit separator (0x1f): Git permits `|`
// in author names, which would corrupt author/date attribution in audit rows.
const FIELD_SEP: char = '\u{1f}';
const COMMIT_PREFIX: &str = "COMMIT\u{1f}";

/// Runs `git log --name-only` in `root` and returns the raw output bytes.
pub fn batched_git_log(root: &str) -> Option<Vec<u8>> {
    let out = Command::new("git")
        .args([
            "-C",
            root,
            "log",
            "--name-only",
            "--format=COMMIT%x1f%H%x1f%an%x1f%aI",
        ])
        .output()
        .ok()?;
    if out.status.success() {
        Some(out.stdout)
    } else {
        None
    }
}

/// Parses a `COMMIT<US>hash<US>author<US>date` formatted line into a `GitMeta`.
pub fn parse_commit_line(line: &str) -> Option<GitMeta> {
    let rest = line.strip_prefix(COMMIT_PREFIX)?;
    let (hash, rest) = rest.split_once(FIELD_SEP)?;
    let (author, date) = rest.split_once(FIELD_SEP)?;
    if hash.is_empty() {
        return None;
    }
    Some(GitMeta {
        hash: hash.to_string(),
        author: author.to_string(),
        date: date.to_string(),
    })
}

/// Normalises a git path by converting backslashes to forward slashes.
pub fn normalize_git_path(p: &str) -> String {
    p.replace('\\', "/")
}

/// Returns `GitMeta` for the most recent commit that touched `abs_path`.
pub fn git_info_file(abs_path: &str) -> Option<GitMeta> {
    let path = std::path::Path::new(abs_path);
    let dir = path.parent()?.to_str()?;
    let name = path.file_name()?.to_str()?;
    let out = Command::new("git")
        .args(["-C", dir, "log", "-1", "--format=%H%n%an%n%aI", "--", name])
        .output()
        .ok()?;
    if !out.status.success() {
        return None;
    }
    let text = String::from_utf8_lossy(&out.stdout);
    let lines: Vec<_> = text.trim().lines().collect();
    if lines.len() < 3 {
        return None;
    }
    Some(GitMeta {
        hash: lines[0].to_string(),
        author: lines[1].to_string(),
        date: lines[2].to_string(),
    })
}

#[cfg(test)]
#[path = "../tests/git_log_test.rs"]
mod git_log_tests;
