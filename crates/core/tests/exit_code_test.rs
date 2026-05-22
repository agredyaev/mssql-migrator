use migrator_core::error::{
    Error, EXIT_CONFIG, EXIT_CONN, EXIT_INVALID_INPUT, EXIT_LOCK_TIMEOUT, EXIT_PLAN_BLOCKED,
    EXIT_SQL,
};

#[test]
fn maps_errors_to_go_exit_codes() {
    assert_eq!(Error::Config("x".into()).exit_code(), EXIT_CONFIG);
    assert_eq!(
        Error::InvalidInput("x".into()).exit_code(),
        EXIT_INVALID_INPUT
    );
    assert_eq!(Error::PlanBlocked.exit_code(), EXIT_PLAN_BLOCKED);
    assert_eq!(Error::LockTimeout.exit_code(), EXIT_LOCK_TIMEOUT);
    assert_eq!(
        Error::Sql("connect localhost:1433: refused".into()).exit_code(),
        EXIT_CONN
    );
    assert_eq!(Error::Sql("syntax error".into()).exit_code(), EXIT_SQL);
}
