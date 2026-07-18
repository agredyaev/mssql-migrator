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

/// Database absence must be recognized only from SQL Server's own "cannot
/// open database" text; digits in a hostname or unrelated failures must
/// propagate as infrastructure errors.
#[test]
fn missing_db_classifier_matches_only_sql_4060_text() {
    use super::is_missing_db_error;
    assert!(is_missing_db_error(
        "Cannot open database \"dactests\" requested by the login. The login failed."
    ));
    assert!(!is_missing_db_error(
        "connection refused: sql4060.internal:1433"
    ));
    assert!(!is_missing_db_error("Login failed for user 'sa'."));
}

#[tokio::test]
async fn create_database_step_is_bounded_regression() {
    let err = super::with_create_database_timeout(
        std::time::Duration::from_millis(1),
        "dactests",
        std::future::pending::<crate::error::Result<()>>(),
    )
    .await
    .expect_err("stalled CREATE DATABASE must time out");
    let msg = err.to_string();
    assert!(msg.contains("CREATE DATABASE dactests timed out"), "{msg}");
}
