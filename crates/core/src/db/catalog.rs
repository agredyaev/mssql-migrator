//! Catalog SQL batch construction and result-row merging.
//!
//! ### Purpose
//! Builds parameterised T-SQL batches for inspecting the database catalog
//! (schemas, sys.objects, types, indexes) and merges the result rows into a
//! [`CatalogState`].

use crate::db::state::{catalog_object, CatalogState};
use crate::domain::ObjectKey;
use crate::error::Result;
use crate::sql;

/// Catalog section for plan batch: scope=@p2, schemas=@p3 (checksums use @p1).
pub fn build_catalog_sql_batch(kinds: &[&str], skip_schema_rows: bool) -> String {
    let header = sql::catalog::SCOPE_HEADER
        .replace("@p2", "@p3")
        .replace("@p1", "@p2");
    build_catalog_from_header(&header, kinds, skip_schema_rows)
}

fn build_catalog_from_header(header: &str, kinds: &[&str], skip_schema_rows: bool) -> String {
    let mut b = String::from(header);
    if !skip_schema_rows {
        b.push_str(", schema_rows AS (");
        b.push_str(sql::catalog::SCHEMA_ROWS);
        b.push(')');
    }
    let has_sys = kinds.iter().any(|k| !matches!(*k, "types" | "indexes"));
    let has_types = kinds.contains(&"types");
    let has_indexes = kinds.contains(&"indexes");
    if has_sys {
        b.push_str(", sys_object_rows AS (");
        b.push_str(sql::catalog::SYS_OBJECTS);
        b.push(')');
    }
    if has_types {
        b.push_str(", type_rows AS (");
        b.push_str(sql::catalog::TYPES);
        b.push(')');
    }
    if has_indexes {
        b.push_str(", index_rows AS (");
        b.push_str(sql::catalog::INDEXES);
        b.push(')');
    }
    let mut first = true;
    let mut push_source = |name: &str| {
        if first {
            b.push_str(" SELECT row_kind, schema_name, kind, object_name, parent_name FROM ");
            b.push_str(name);
            first = false;
        } else {
            b.push_str(
                " UNION ALL SELECT row_kind, schema_name, kind, object_name, parent_name FROM ",
            );
            b.push_str(name);
        }
    };
    if !skip_schema_rows {
        push_source("schema_rows");
    }
    if has_sys {
        push_source("sys_object_rows");
    }
    if has_types {
        push_source("type_rows");
    }
    if has_indexes {
        push_source("index_rows");
    }
    b
}

/// Heuristic: true when rows look like catalog inspect output (first cell is `"object"` or `"schema"`).
pub fn looks_like_catalog_rows(rows: &[crate::driver::RowData]) -> bool {
    rows.first()
        .is_some_and(|r| r.cells.len() >= 5 && matches!(r.get_str(0), Some("object" | "schema")))
}

/// Heuristic: true when rows look like cache-load output (first cell contains `/`).
pub fn looks_like_cache_load_rows(rows: &[crate::driver::RowData]) -> bool {
    rows.first()
        .is_some_and(|r| r.cells.len() >= 5 && r.get_str(0).is_some_and(|k| k.contains('/')))
}

/// Scoped hit for plan batch (scope=@p2 when checksums use @p1).
pub fn scoped_hit_sql_batch() -> String {
    sql::catalog::SCOPED_HIT.replace("@p1", "@p2")
}

/// Merge catalog SQL result rows into a [`CatalogState`].
pub fn merge_rows(state: &mut CatalogState, rows: &[crate::driver::RowData]) -> Result<()> {
    for row in rows {
        let kind = row.get_str(0).unwrap_or("");
        if kind == "schema" {
            let schema = row.get_str(1).unwrap_or("");
            state.schemas.insert(schema.to_lowercase());
            continue;
        }
        let schema = row.get_str(1).unwrap_or("");
        let obj_kind = row.get_str(2).unwrap_or("");
        let name = row.get_str(3).unwrap_or("");
        let parent = row.get_str(4);
        let key = ObjectKey::new(schema, obj_kind, name);
        super::catalog_guard::ensure_index_unambiguous(state, &key, obj_kind, parent)?;
        state
            .objects
            .insert(key, catalog_object(schema, obj_kind, name, parent));
    }
    Ok(())
}

#[cfg(test)]
#[path = "tests/catalog.rs"]
mod catalog_tests;
