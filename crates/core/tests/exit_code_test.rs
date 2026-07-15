use migrator_core::error::{
    Error, EXIT_CONFIG, EXIT_CONN, EXIT_INVALID_INPUT, EXIT_LOCK_TIMEOUT, EXIT_PLAN_BLOCKED,
    EXIT_SQL,
};

#[test]
fn connect_error_maps_to_exit_conn_happy_path() {
    assert_eq!(
        Error::Conn("connect localhost:1433: refused".into()).exit_code(),
        EXIT_CONN
    );
}

#[test]
fn runtime_sql_error_maps_to_exit_sql_negative_path() {
    // A SQL error whose message happens to contain "connect" must NOT be
    // misclassified as a connection failure (the old substring heuristic did).
    assert_eq!(
        Error::Sql("connect_users proc failed".into()).exit_code(),
        EXIT_SQL
    );
    assert_eq!(Error::Sql("syntax error".into()).exit_code(), EXIT_SQL);
}

#[test]
fn tds_handshake_error_maps_to_exit_conn_edge_case() {
    assert_eq!(
        Error::Conn("connect localhost:1433: tds handshake: login failed".into()).exit_code(),
        EXIT_CONN
    );
}

#[test]
fn connection_phase_failures_use_dedicated_variant_regression() {
    // BG-008: connection-phase failures must map to EXIT_CONN. Now carried by
    // the typed Error::Conn variant instead of message substring sniffing.
    assert_eq!(
        Error::Conn("tds handshake: token failure".into()).exit_code(),
        EXIT_CONN,
        "BG-008 regression: handshake failures must map to EXIT_CONN"
    );
}

#[test]
fn maps_non_sql_errors_to_documented_exit_codes() {
    assert_eq!(Error::Config("x".into()).exit_code(), EXIT_CONFIG);
    assert_eq!(
        Error::InvalidInput("x".into()).exit_code(),
        EXIT_INVALID_INPUT
    );
    assert_eq!(Error::PlanBlocked.exit_code(), EXIT_PLAN_BLOCKED);
    assert_eq!(Error::LockTimeout.exit_code(), EXIT_LOCK_TIMEOUT);
}
