//! Offline drift pins: checksum semantics are raw file bytes, so a pure
//! line-ending flip on a transition-less table blocks the whole plan.

use migrator_core::db::state::{catalog_object, CatalogState, ChecksumMap};
use migrator_core::domain::Workspace;
use migrator_core::plan::compute_diff;
use migrator_core::scan::scan_root;

/// CRLF flip changes the raw-byte SHA-256, which the diff treats as a real
/// table change; without transitions the plan is blocked (availability pin).
#[test]
fn crlf_flip_blocks_table() {
    let body_lf = "CREATE TABLE smoke.t1 (\n    id INT NOT NULL\n);\n";
    let body_crlf = body_lf.replace('\n', "\r\n");

    let checksum_lf = scan_checksum(body_lf);
    let checksum_crlf = scan_checksum(body_crlf.as_str());
    assert_ne!(
        checksum_lf, checksum_crlf,
        "line-ending flip must change the raw-byte checksum"
    );

    // Repo now holds the CRLF file; history recorded the LF checksum.
    let dir = tempfile::tempdir().expect("tempdir");
    let mut ws = write_and_scan(dir.path(), body_crlf.as_str());
    let key = ws.entry_key(0).clone();
    let mut checksums = ChecksumMap::new();
    checksums.insert_key(&key, checksum_lf);
    let mut catalog = CatalogState::default();
    catalog
        .objects
        .insert(key, catalog_object("smoke", "tables", "t1", None));

    let (plan, _) = compute_diff(&mut ws, &catalog, &checksums).expect("diff");
    assert!(
        plan.blocked,
        "CRLF-flipped table without transitions must block"
    );
    assert_eq!(plan.summary.blocked_count, 1);
}

fn scan_checksum(body: &str) -> [u8; 32] {
    let dir = tempfile::tempdir().expect("tempdir");
    let ws = write_and_scan(dir.path(), body);
    ws.entry(0).checksum
}

fn write_and_scan(root: &std::path::Path, body: &str) -> Workspace {
    let table_dir = root.join("dactests/smoke/tables");
    std::fs::create_dir_all(&table_dir).expect("layout dirs");
    std::fs::write(table_dir.join("t1.sql"), body).expect("write script");
    let mut ws = Workspace::default();
    scan_root(&mut ws, root.to_str().expect("utf8 root")).expect("scan");
    assert_eq!(ws.object_count(), 1, "single scanned object");
    ws
}
