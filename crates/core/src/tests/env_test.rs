use std::io::Write;

use crate::config::env::{load_env_file, load_env_file_required};
use crate::config::env_build::{parse_bool, parse_duration};
use proptest::prelude::*;

#[test]
fn load_env_file_reads_existing_file_happy_path() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("ok.env");
    std::fs::write(&path, "RM_SQL_ROOT=/data/sql\n").expect("write env");
    let env = load_env_file_required(&path).expect("load env");
    assert_eq!(
        env.get("RM_SQL_ROOT").map(String::as_str),
        Some("/data/sql")
    );
}

#[test]
fn load_env_file_required_missing_file_negative_path() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("missing.env");
    let err = load_env_file_required(&path).expect_err("missing env should fail");
    assert!(
        err.to_string().contains("env file not found"),
        "unexpected error: {err}"
    );
}

#[test]
fn load_env_file_optional_missing_file_edge_case() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("missing.env");
    let env = load_env_file(&path).expect("optional missing env should be empty");
    assert!(env.is_empty());
}

#[test]
fn load_env_file_required_missing_file_regression() {
    let dir = tempfile::tempdir().expect("tempdir");
    let path = dir.path().join("does-not-exist.env");
    std::env::set_var("RM_DB_SERVER", "ambient-should-not-mask-missing-env");
    let err = load_env_file_required(&path).expect_err("explicit env path must fail fast");
    std::env::remove_var("RM_DB_SERVER");
    assert!(
        err.to_string().contains("does-not-exist.env"),
        "unexpected error: {err}"
    );
}

#[test]
fn load_env_file_unreadable_file_negative_path() {
    #[cfg(unix)]
    {
        let dir = tempfile::tempdir().expect("tempdir");
        let path = dir.path().join("locked.env");
        {
            let mut f = std::fs::File::create(&path).expect("create env");
            writeln!(f, "RM_SQL_ROOT=/data/sql").expect("write env");
        }
        use std::os::unix::fs::PermissionsExt;
        std::fs::set_permissions(&path, std::fs::Permissions::from_mode(0o000)).expect("chmod 000");
        let err = load_env_file_required(&path).expect_err("unreadable env should fail");
        std::fs::set_permissions(&path, std::fs::Permissions::from_mode(0o600))
            .expect("restore perms");
        assert!(
            err.to_string().contains("env file unreadable"),
            "unexpected error: {err}"
        );
    }
}

#[test]
fn parse_bool_accepts_case_insensitive_truthy_values() {
    for raw in ["1", "true", "TRUE", "Yes", " yes "] {
        assert!(parse_bool(raw), "expected truthy parse for {raw:?}");
    }
}

#[test]
fn parse_bool_rejects_non_truthy_values() {
    for raw in ["", "0", "false", "no", "tru", "  "] {
        assert!(!parse_bool(raw), "expected false parse for {raw:?}");
    }
}

#[test]
fn parse_duration_accepts_integer_seconds_happy_path() {
    assert_eq!(
        parse_duration("15").expect("integer seconds should parse"),
        std::time::Duration::from_secs(15)
    );
}

#[test]
fn parse_duration_accepts_fractional_seconds_edge_case() {
    assert_eq!(
        parse_duration("1.5s").expect("fractional seconds should parse"),
        std::time::Duration::from_millis(1500)
    );
}

#[test]
fn parse_duration_rejects_invalid_suffix_negative_path() {
    let err = parse_duration("1ms").expect_err("unsupported suffix should fail");
    assert!(err.to_string().contains("1ms"), "unexpected error: {err}");
}

proptest! {
    #[test]
    fn parse_duration_integer_round_trip_fuzz(secs in 0u16..10_000) {
        let parsed = parse_duration(&secs.to_string()).expect("integer seconds should parse");
        prop_assert_eq!(parsed, std::time::Duration::from_secs(u64::from(secs)));
    }
}
