//! In-memory catalog state loaded from SQL Server inspection queries.
//!
//! ### Purpose
//! [`CatalogState`] holds the set of existing schemas and objects discovered
//! during `db::inspect_with_scope`. [`CatalogObject`] is a single row with
//! arena-backed `SharedStr` fields. Helper functions build objects from raw
//! wire strings and intern the entire state into the domain string arena.

use std::collections::{HashMap, HashSet};

use serde::{Deserialize, Serialize};

use crate::domain::{share, ObjectKey, SharedStr};

pub use super::ChecksumMap;
pub use crate::domain::key_fingerprint;

/// Full catalog snapshot: known schemas and objects keyed by [`ObjectKey`].
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
pub struct CatalogState {
    /// Set of schema names present in the database.
    pub schemas: HashSet<String>,
    /// Map of object key → object metadata.
    pub objects: HashMap<ObjectKey, CatalogObject>,
}

/// A single catalog object row with arena-shared string fields.
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct CatalogObject {
    /// SQL schema name (arena-backed).
    pub schema: SharedStr,
    /// Object kind (`tables`, `views`, `procedures`, …; arena-backed).
    pub kind: SharedStr,
    /// SQL object name (arena-backed).
    pub name: SharedStr,
    /// Parent object name (e.g. table for an index; arena-backed, `None` for top-level).
    pub parent: Option<SharedStr>,
}

/// A single table column descriptor from the SQL type/index inspect queries.
#[derive(Clone, Debug)]
pub struct TableColumn {
    /// Column name.
    pub name: String,
    /// SQL type name (e.g. `nvarchar`, `int`).
    pub type_name: String,
    /// True when the column allows NULL.
    pub nullable: bool,
}

/// Build a catalog row from SQL inspect wire strings (deduped via `share`).
pub fn catalog_object(schema: &str, kind: &str, name: &str, parent: Option<&str>) -> CatalogObject {
    let parent = parent.filter(|p| !p.is_empty()).map(share);
    catalog_object_parts(share(schema), share(kind), share(name), parent)
}

/// Build a catalog row reusing layout `SharedStr` (no duplicate `share()` / Arc).
pub fn catalog_object_parts(
    schema: SharedStr,
    kind: SharedStr,
    name: SharedStr,
    parent: Option<SharedStr>,
) -> CatalogObject {
    CatalogObject {
        schema,
        kind,
        name,
        parent,
    }
}

/// Derive catalog columns as arena subslices of the normalized key.
pub fn catalog_object_from_key(key: &ObjectKey) -> CatalogObject {
    CatalogObject {
        schema: key.schema_shared(),
        kind: key.kind_shared(),
        name: key.name_shared(),
        parent: None,
    }
}

/// Deduplicate catalog wire strings via domain arena (called after SQL load).
pub fn intern_catalog_state(state: &mut CatalogState) {
    if state.objects.is_empty() && state.schemas.is_empty() {
        return;
    }
    use crate::domain::StringArenaBuilder;
    let mut builder = StringArenaBuilder::with_capacity(
        state.objects.len() * 48 + state.schemas.len() * 16,
        state.objects.len() + state.schemas.len(),
    );
    for schema in &state.schemas {
        builder.register(schema);
    }
    for obj in state.objects.values() {
        builder.register(obj.schema.as_ref());
        builder.register(obj.kind.as_ref());
        builder.register(obj.name.as_ref());
        if let Some(parent) = &obj.parent {
            builder.register(parent.as_ref());
        }
    }
    let arena = builder.finish();
    state.schemas = state
        .schemas
        .iter()
        .map(|s| arena.get(s).as_ref().to_string())
        .collect();
    for obj in state.objects.values_mut() {
        obj.schema = arena.get(obj.schema.as_ref());
        obj.kind = arena.get(obj.kind.as_ref());
        obj.name = arena.get(obj.name.as_ref());
        if let Some(parent) = obj.parent.take() {
            obj.parent = Some(arena.get(parent.as_ref()));
        }
    }
}
