#![cfg(unix)]

use std::os::unix::fs::PermissionsExt;

use super::ensure_private_parent;

/// A pre-existing caller-owned parent must never be chmodded; loose modes are
/// refused instead of silently hardened.
#[test]
fn existing_shared_parent_is_refused_and_untouched_regression() {
    let dir = tempfile::tempdir().expect("tempdir");
    std::fs::set_permissions(dir.path(), std::fs::Permissions::from_mode(0o755)).expect("chmod");

    let err = ensure_private_parent(dir.path()).expect_err("0755 parent must be refused");
    assert!(err.to_string().contains("group/world"), "error: {err}");

    let mode = std::fs::metadata(dir.path())
        .expect("stat")
        .permissions()
        .mode()
        & 0o777;
    assert_eq!(mode, 0o755, "pre-existing parent mode must stay untouched");
}

#[test]
fn existing_private_parent_is_accepted_happy_path() {
    let dir = tempfile::tempdir().expect("tempdir");
    std::fs::set_permissions(dir.path(), std::fs::Permissions::from_mode(0o700)).expect("chmod");
    ensure_private_parent(dir.path()).expect("0700 parent is fine");
}

#[test]
fn new_parent_is_created_private_happy_path() {
    let base = tempfile::tempdir().expect("tempdir");
    let parent = base.path().join("fresh");
    ensure_private_parent(&parent).expect("create");
    let mode = std::fs::metadata(&parent)
        .expect("stat")
        .permissions()
        .mode()
        & 0o777;
    assert_eq!(mode, 0o700, "created parent must be private");
}
