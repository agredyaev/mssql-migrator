use crate::db::state::CatalogState;
use crate::domain::ObjectKey;
use crate::error::{Error, Result};

/// SQL Server allows the same index name on different tables, but the
/// repository key carries no parent — refusing the ambiguity beats
/// order-dependently associating the index with the wrong table.
pub(super) fn ensure_index_unambiguous(
    state: &CatalogState,
    key: &ObjectKey,
    obj_kind: &str,
    parent: Option<&str>,
) -> Result<()> {
    if obj_kind != "indexes" {
        return Ok(());
    }
    let Some(prev) = state.objects.get(key) else {
        return Ok(());
    };
    let prev_parent = prev.parent.as_deref().unwrap_or("");
    let new_parent = parent.unwrap_or("");
    if prev_parent == new_parent {
        return Ok(());
    }
    Err(Error::InvalidInput(format!(
        "ambiguous index {}: exists on both '{prev_parent}' and '{new_parent}'; \
         index names managed by the repository must be unique per schema",
        key.as_str()
    )))
}

#[cfg(test)]
#[path = "../tests/catalog_guard_test.rs"]
mod catalog_guard_tests;
