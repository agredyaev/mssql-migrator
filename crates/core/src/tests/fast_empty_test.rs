use super::fast_empty_provable;

/// A zero-hit sys.objects probe must not prove emptiness for scopes
/// containing indexes or table types — those live in other catalog views.
#[test]
fn fast_empty_not_provable_when_scope_has_indexes_or_types_regression() {
    assert!(fast_empty_provable(&["views", "tables"]));
    assert!(fast_empty_provable(&[]));
    assert!(!fast_empty_provable(&["views", "indexes"]));
    assert!(!fast_empty_provable(&["types"]));
}
