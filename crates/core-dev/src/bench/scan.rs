use std::fs;
use std::path::{Path, PathBuf};

use migrator_core::domain::Workspace;
use migrator_core::scan::scan_root;

pub fn scan_fixture_workspace(root: &Path, n: usize) -> Workspace {
    fs::create_dir_all(root.join("testdb/schema/views")).expect("mkdir views");
    for i in 0..n {
        let path = root.join(format!("testdb/schema/views/obj_{i}.sql"));
        fs::write(
            &path,
            format!("CREATE VIEW [schema].[obj_{i}] AS SELECT 1 AS x\n"),
        )
        .expect("write sql");
    }
    let mut ws = Workspace::default();
    scan_root(&mut ws, root.to_str().expect("utf8 root")).expect("scan_root");
    ws
}

pub fn temp_scan_root(prefix: &str) -> PathBuf {
    let dir = std::env::temp_dir().join(format!("rmig_scan_bench_{prefix}_{}", std::process::id()));
    let _ = fs::remove_dir_all(&dir);
    dir
}
