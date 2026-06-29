use super::*;

#[test]
fn classified_errors_map_to_documented_codes() {
    assert_eq!(Error::Config("x".into()).exit_code(), EXIT_CONFIG);
    assert_eq!(
        Error::InvalidInput("x".into()).exit_code(),
        EXIT_INVALID_INPUT
    );
    assert_eq!(Error::PlanBlocked.exit_code(), EXIT_PLAN_BLOCKED);
    assert_eq!(Error::LockTimeout.exit_code(), EXIT_LOCK_TIMEOUT);
}

#[test]
fn sql_errors_split_connect_from_execution() {
    // Connection-phase failures are surfaced as a distinct exit code so CI can
    // retry infrastructure issues separately from genuine SQL errors.
    assert_eq!(
        Error::Sql("connect timed out".into()).exit_code(),
        EXIT_CONN
    );
    assert_eq!(
        Error::Sql("TDS handshake failed".into()).exit_code(),
        EXIT_CONN
    );
    assert_eq!(
        Error::Sql("Invalid column name 'foo'".into()).exit_code(),
        EXIT_SQL
    );
}

#[test]
fn io_and_other_are_general() {
    let io = Error::Io(std::io::Error::other("boom"));
    assert_eq!(io.exit_code(), EXIT_GENERAL);
    assert_eq!(Error::Other(anyhow::anyhow!("x")).exit_code(), EXIT_GENERAL);
}
