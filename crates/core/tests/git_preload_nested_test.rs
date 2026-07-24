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

#[tokio::test]
async fn scan_preloads_git_with_nested_sql_root() {
    let tmp = tempfile::tempdir().expect("tempdir");
    let repo = tmp.path();
    init_git_repo(repo);
    let sql_root = repo.join(".temp/sql");
    let sql_path = sql_root.join("db/sch/views/monthly.sql");
    fs::create_dir_all(sql_path.parent().unwrap()).expect("mkdir");
    fs::write(&sql_path, "SELECT 1\n").expect("write");
    commit_all(repo, "c1");

    let mut ws = Workspace::default();
    scan::populate(&mut ws, sql_root.to_str().unwrap(), false)
        .await
        .expect("scan");
    let sk = migrator_core::domain::ScriptKey::from_path("db/sch/views/monthly.sql");
    let script = ws.script_by_key(&sk).expect("script");
    assert_eq!(script.git_hash().len(), 40);
}
