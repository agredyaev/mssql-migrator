use super::ensure_index_unambiguous;
use crate::db::state::{catalog_object, CatalogState};
use crate::domain::ObjectKey;

/// The same index name on two different parent tables must be refused, not
/// order-dependently matched to whichever catalog row arrived first.
#[test]
fn duplicate_index_name_with_different_parent_is_ambiguous_regression() {
    let mut state = CatalogState::default();
    let key = ObjectKey::new("dbo", "indexes", "ix_name");
    state.objects.insert(
        key.clone(),
        catalog_object("dbo", "indexes", "ix_name", Some("t1")),
    );

    let err = ensure_index_unambiguous(&state, &key, "indexes", Some("t2"))
        .expect_err("same index name on a different parent must be ambiguous");
    assert!(err.to_string().contains("ambiguous index"), "got: {err}");

    ensure_index_unambiguous(&state, &key, "indexes", Some("t1")).expect("same parent is fine");
    ensure_index_unambiguous(&state, &key, "tables", Some("t2")).expect("non-index kind ignored");
}
