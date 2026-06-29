use super::ensure_catalog_databases_exist;
use crate::config::Config;
use crate::error::EXIT_INVALID_INPUT;

// Validation runs before any connection, so these never touch SQL Server.

#[tokio::test]
async fn rejects_overlong_catalog_name() {
    let cfg = Config::default();
    let names = vec!["a".repeat(129)];
    let err = ensure_catalog_databases_exist(&cfg, &names)
        .await
        .expect_err("129-char name exceeds the 128-char identifier limit");
    assert_eq!(err.exit_code(), EXIT_INVALID_INPUT, "got: {err}");
}

#[tokio::test]
async fn rejects_path_traversal_catalog_name() {
    let cfg = Config::default();
    for bad in ["..", "a/b", "a\\b"] {
        let err = ensure_catalog_databases_exist(&cfg, &[bad.to_string()])
            .await
            .expect_err("illegal identifier must be rejected before any DB call");
        assert_eq!(err.exit_code(), EXIT_INVALID_INPUT, "{bad:?} -> {err}");
    }
}
