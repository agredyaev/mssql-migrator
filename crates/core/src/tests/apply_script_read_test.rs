use super::verified_body;

#[test]
fn verified_body_accepts_matching_checksum_happy_path() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("v.sql");
    let body = "CREATE OR ALTER VIEW dbo.v AS SELECT 1 AS one";
    std::fs::write(&path, body).expect("write");
    let expected = crate::scan::content_checksum(body.as_bytes());
    let got = verified_body(path.to_str().unwrap(), &expected, "dbo/views/v").expect("verified");
    assert_eq!(got, body);
}

/// The plan's checksum was computed at scan time; a file whose bytes changed
/// before apply must be refused before any SQL executes.
#[test]
fn verified_body_rejects_changed_file_regression() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("v.sql");
    std::fs::write(&path, "CREATE OR ALTER VIEW dbo.v AS SELECT 1 AS one").expect("write");
    let scanned = crate::scan::content_checksum(b"CREATE OR ALTER VIEW dbo.v AS SELECT 2 AS two");
    let err = verified_body(path.to_str().unwrap(), &scanned, "dbo/views/v")
        .expect_err("changed bytes must be refused");
    assert!(err.contains("changed after scan"), "message: {err}");
}

#[test]
fn verified_body_reports_missing_file_edge_case() {
    let err = verified_body("/nonexistent/x.sql", &[0u8; 32], "dbo/views/x")
        .expect_err("missing file must error");
    assert!(err.contains("read failed"), "message: {err}");
}
