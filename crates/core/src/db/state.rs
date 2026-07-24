//! In-memory catalog state loaded from SQL Server inspection queries.
//!
//! ### Purpose
//! [`CatalogState`] holds the set of existing schemas and objects discovered
//! during `db::inspect_with_scope`. [`CatalogObject`] is a single row with
//! owned string fields. Helper functions build objects from raw wire strings.

use std::collections::{HashMap, HashSet};

use serde::{Deserialize, Serialize};

use crate::domain::ObjectKey;

pub use super::ChecksumMap;

/// Full catalog snapshot: known schemas and objects keyed by [`ObjectKey`].
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
pub struct CatalogState {
    /// Set of schema names present in the database.
    pub schemas: HashSet<String>,
    /// Map of object key → object metadata.
    pub objects: HashMap<ObjectKey, CatalogObject>,
}

/// A single catalog object row.
#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct CatalogObject {
    /// SQL schema name.
    pub schema: String,
    /// Object kind (`tables`, `views`, `procedures`, …).
    pub kind: String,
    /// SQL object name.
    pub name: String,
    /// Parent object name, or `None` for top-level objects.
    pub parent: Option<String>,
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

/// Builds a catalog row from SQL inspect strings.
pub fn catalog_object(schema: &str, kind: &str, name: &str, parent: Option<&str>) -> CatalogObject {
    let parent = parent.filter(|p| !p.is_empty()).map(str::to_owned);
    catalog_object_parts(schema.to_owned(), kind.to_owned(), name.to_owned(), parent)
}

/// Builds a catalog row from owned parts.
pub fn catalog_object_parts(
    schema: String,
    kind: String,
    name: String,
    parent: Option<String>,
) -> CatalogObject {
    CatalogObject {
        schema,
        kind,
        name,
        parent,
    }
}

/// Derives catalog columns from the normalized key.
pub fn catalog_object_from_key(key: &ObjectKey) -> CatalogObject {
    CatalogObject {
        schema: key.schema_shared(),
        kind: key.kind_shared(),
        name: key.name_shared(),
        parent: None,
    }
}
