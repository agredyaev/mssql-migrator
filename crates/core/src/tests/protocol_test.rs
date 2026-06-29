use super::Request;

#[test]
fn debug_redacts_auth_token() {
    let req = Request::Auth {
        token: "super-secret-token".into(),
    };
    let dbg = format!("{req:?}");
    assert!(!dbg.contains("super-secret-token"), "token leaked: {dbg}");
    assert!(
        dbg.contains("<redacted>"),
        "expected redaction marker: {dbg}"
    );
}

#[test]
fn debug_keeps_sql_visible_for_diagnostics() {
    let req = Request::Exec {
        sql: "SELECT 1".into(),
    };
    assert!(
        format!("{req:?}").contains("SELECT 1"),
        "non-secret SQL should stay visible"
    );
}
