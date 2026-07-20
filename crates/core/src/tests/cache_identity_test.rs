use super::db_fingerprint;

/// Two SQL Server instances differing only by port (or principal) must never
/// share plan-cache identity.
#[test]
fn fingerprint_separates_port_and_user_regression() {
    let base = db_fingerprint("localhost", "1433", "sa", "reporting");
    assert_ne!(
        base,
        db_fingerprint("localhost", "1434", "sa", "reporting"),
        "different port is a different endpoint"
    );
    assert_ne!(
        base,
        db_fingerprint("localhost", "1433", "deploy", "reporting"),
        "different principal sees different metadata"
    );
    assert_ne!(
        base,
        db_fingerprint("localhost", "1433", "sa", "other"),
        "different database"
    );
}

#[test]
fn fingerprint_length_prefix_prevents_separator_collisions_edge_case() {
    assert_ne!(
        db_fingerprint("s1", "1433", "sa", "a~b"),
        db_fingerprint("s1~1433", "sa", "a", "b"),
    );
}
