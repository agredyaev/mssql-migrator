use std::fs;
use std::process::Command;

use migrator_core::domain::Workspace;
use migrator_core::scan;

fn init_git_repo(dir: &std::path::Path) {
    for args in [
        &["init"][..],
        &["config", "user.email", "t@example.com"][..],
        &["config", "user.name", "test"][..],
    ] {
        let status = Command::new("git")
            .arg("-C")
            .arg(dir)
            .args(args)
            .status()
            .expect("git");
        assert!(status.success(), "git {:?}", args);
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

#[tokio::test]
async fn scan_preloads_git_metadata() {
    let tmp = tempfile::tempdir().expect("tempdir");
    let root = tmp.path();
    init_git_repo(root);
    let sql_path = root.join("db/sch/views/monthly.sql");
    fs::create_dir_all(sql_path.parent().unwrap()).expect("mkdir");
    fs::write(&sql_path, "SELECT 1\n").expect("write");
    commit_all(root, "c1");

    let mut ws = Workspace::default();
    scan::populate(&mut ws, root.to_str().unwrap(), false)
        .await
        .expect("scan");
    let script = ws
        .script_by_key(&migrator_core::domain::ScriptKey::from_path(
            "db/sch/views/monthly.sql",
        ))
        .expect("script");
    assert_eq!(script.git_hash().as_ref().len(), 40);
    assert!(!script.git_author().is_empty());
    assert!(!script.git_date().is_empty());
}

#[tokio::test]
async fn scan_skips_git_when_requested() {
    let tmp = tempfile::tempdir().expect("tempdir");
    let root = tmp.path();
    init_git_repo(root);
    fs::create_dir_all(root.join("db/sch/tables")).expect("mkdir");
    fs::write(root.join("db/sch/tables/t.sql"), "CREATE TABLE t(x int)\n").expect("write");
    commit_all(root, "c1");

    let mut ws = Workspace::default();
    scan::populate(&mut ws, root.to_str().unwrap(), true)
        .await
        .expect("scan");
    let script = ws.scripts_iter().next().expect("script");
    assert!(script.git_hash().is_empty());
}
