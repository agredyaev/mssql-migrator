use std::fs;
use std::process::Command;

use migrator_core::gate::resolve_changed_paths;

fn init_git_repo(dir: &std::path::Path) {
    for args in [
        &["init"][..],
        &["config", "user.email", "t@example.com"][..],
        &["config", "user.name", "test"][..],
    ] {
        Command::new("git")
            .arg("-C")
            .arg(dir)
            .args(args)
            .status()
            .expect("git");
    }
}

fn commit_all(dir: &std::path::Path, msg: &str) {
    Command::new("git")
        .args(["-C", dir.to_str().unwrap(), "add", "."])
        .status()
        .expect("git add");
    Command::new("git")
        .args(["-C", dir.to_str().unwrap(), "commit", "-m", msg])
        .status()
        .expect("git commit");
}

#[test]
fn resolve_changed_paths_no_git_full_inspect() {
    let tmp = tempfile::tempdir().expect("tempdir");
    let res = resolve_changed_paths(tmp.path().to_str().unwrap());
    assert!(res.full_inspect);
    assert_eq!(res.source, "no-git");
}

#[test]
fn resolve_changed_paths_git_diff() {
    let tmp = tempfile::tempdir().expect("tempdir");
    let dir = tmp.path();
    init_git_repo(dir);
    fs::write(dir.join("a.sql"), "v1\n").expect("write");
    commit_all(dir, "c1");
    Command::new("git")
        .args(["-C", dir.to_str().unwrap(), "branch", "-M", "main"])
        .status()
        .expect("branch");
    Command::new("git")
        .args(["-C", dir.to_str().unwrap(), "checkout", "-b", "feature"])
        .status()
        .expect("checkout");
    fs::write(dir.join("b.sql"), "v2\n").expect("write");
    commit_all(dir, "c2");

    let res = resolve_changed_paths(dir.to_str().unwrap());
    assert!(!res.full_inspect, "source={}", res.source);
    assert!(
        res.paths.iter().any(|p| p == "b.sql"),
        "paths={:?} source={}",
        res.paths,
        res.source
    );
}

#[test]
fn parse_commit_line_matches_batched_format() {
    use migrator_core::git::parse_commit_line;
    let m =
        parse_commit_line("COMMIT\u{1f}abc123\u{1f}Jane\u{1f}2026-01-02T15:04:05Z").expect("meta");
    assert_eq!(m.hash, "abc123");
    assert_eq!(m.author, "Jane");
    assert_eq!(m.date, "2026-01-02T15:04:05Z");
}
