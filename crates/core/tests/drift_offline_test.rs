//! Offline drift pins: checksums are line-ending-normalized, so a pure
//! CRLF/LF flip is not a change — while a real edit still blocks a
//! transition-less table.

use migrator_core::db::state::{catalog_object, CatalogState, ChecksumMap};
use migrator_core::domain::Workspace;
use migrator_core::plan::compute_diff;
use migrator_core::scan::scan_root;

/// A checkout-only line-ending flip must NOT read as table drift: CRLF folds
/// to LF before hashing, so the plan stays clean and unblocked.
#[test]
fn crlf_flip_is_not_drift_regression() {
    let body_lf = "CREATE TABLE smoke.t1 (\n    id INT NOT NULL\n);\n";
    let body_crlf = body_lf.replace('\n', "\r\n");

    let checksum_lf = scan_checksum(body_lf);
    let checksum_crlf = scan_checksum(body_crlf.as_str());
    assert_eq!(
        checksum_lf, checksum_crlf,
        "line-ending flip must not change the normalized checksum"
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
    assert!(!plan.blocked, "CRLF-only flip must not block the plan");
    assert_eq!(plan.summary.blocked_count, 0);
}

/// A REAL body edit on a transition-less table still blocks (the availability
/// pin the CRLF normalization must not weaken).
#[test]
fn real_table_edit_still_blocks_edge_case() {
    let recorded = scan_checksum("CREATE TABLE smoke.t1 (\n    id INT NOT NULL\n);\n");

    let dir = tempfile::tempdir().expect("tempdir");
    let mut ws = write_and_scan(
        dir.path(),
        "CREATE TABLE smoke.t1 (\n    id INT NOT NULL,\n    extra INT\n);\n",
    );
    let key = ws.entry_key(0).clone();
    let mut checksums = ChecksumMap::new();
    checksums.insert_key(&key, recorded);
    let mut catalog = CatalogState::default();
    catalog
        .objects
        .insert(key, catalog_object("smoke", "tables", "t1", None));

    let (plan, _) = compute_diff(&mut ws, &catalog, &checksums).expect("diff");
    assert!(plan.blocked, "a real change without transitions must block");
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
