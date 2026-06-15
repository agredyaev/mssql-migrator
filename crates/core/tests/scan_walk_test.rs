use migrator_core::domain::Workspace;
use migrator_core::scan::scan_root;

#[test]
fn scan_rejects_backslash_path_component_regression() {
    let base = tempfile::tempdir().expect("tempdir");
    let path = base.path().join("bad\\db/smoke/tables/t1.sql");
    std::fs::create_dir_all(path.parent().expect("parent")).expect("mkdir");
    std::fs::write(&path, "CREATE TABLE smoke.t1(id INT NOT NULL);\n").expect("write sql");

    let mut ws = Workspace::default();
    let err = scan_root(&mut ws, base.path().to_str().expect("utf8 path"))
        .expect_err("backslash inside a path component must not be normalized as a separator");

    assert!(
        err.to_string().contains("invalid character"),
        "unexpected error: {err}"
    );
    assert_eq!(ws.object_count(), 0);
}

#[cfg(unix)]
#[test]
fn scan_skips_symlinked_sql_files() {
    use std::os::unix::fs::symlink;

    let base = tempfile::tempdir().expect("tempdir");
    let root = base.path().join("root");
    let external = base.path().join("external.sql");
    let real = root.join("dactests/smoke/tables/real.sql");
    let link = root.join("dactests/smoke/tables/link.sql");
    std::fs::create_dir_all(real.parent().expect("parent")).expect("mkdir");
    std::fs::write(&real, "CREATE TABLE smoke.real(id INT NOT NULL);\n").expect("write real");
    std::fs::write(&external, "CREATE TABLE smoke.link(id INT NOT NULL);\n").expect("write ext");
    symlink(&external, &link).expect("symlink");

    let mut ws = Workspace::default();
    scan_root(&mut ws, root.to_str().expect("utf8 path")).expect("scan");

    assert_eq!(ws.object_count(), 1);
    assert_eq!(ws.script_count(), 1);
    let paths: Vec<_> = ws
        .scripts_iter()
        .map(|s| s.path_str().to_string())
        .collect();
    assert_eq!(paths, vec!["dactests/smoke/tables/real.sql"]);
}
