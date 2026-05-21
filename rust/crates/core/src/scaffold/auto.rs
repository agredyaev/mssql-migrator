use std::collections::HashMap;
use std::path::Path;

use crate::db::TableColumn;
use crate::error::Result;
use crate::export::PlannedObject;
use crate::sql_ident::{bracket_ident, validate_filename_token, validate_path_component};

use super::column_parser::{has_dropped_columns, new_columns, parse_table_columns, ParsedColumn};
use super::content::scaffold_content;

pub fn try_auto_migration(
    obj: &PlannedObject,
    file_content: &str,
    db_columns: &[TableColumn],
    commit: &str,
    dir: &Path,
) -> Option<(String, String)> {
    validate_filename_token(commit).ok()?;
    let file_cols = parse_table_columns(file_content);
    if file_cols.is_empty() {
        return None;
    }
    let db_names: HashMap<String, bool> =
        db_columns.iter().map(|c| (c.name.clone(), true)).collect();
    let added = new_columns(&file_cols, &db_names);
    if added.is_empty() || has_dropped_columns(&file_cols, &db_names) {
        return None;
    }
    let sql = auto_add_sql(&obj.schema_name, &obj.object_name, &added).ok()?;
    let file_name = format!("001_{commit}_auto_add_columns.sql");
    if dir.join(&file_name).exists() {
        return None;
    }
    Some((sql, file_name))
}

pub fn fallback_scaffold(
    obj: &PlannedObject,
    db_columns: &[TableColumn],
    commit: &str,
) -> (String, String) {
    let commit = validate_filename_token(commit)
        .map(|_| commit.to_string())
        .unwrap_or_else(|_| "0000000".into());
    (
        format!("001_{commit}_describe_change.sql"),
        scaffold_content(&obj.schema_name, &obj.object_name, db_columns),
    )
}

fn auto_add_sql(schema: &str, table: &str, added: &[ParsedColumn]) -> Result<String> {
    let schema_id = bracket_ident(schema)?;
    let table_id = bracket_ident(table)?;
    let mut b = String::new();
    b.push_str(&format!(
        "-- Auto-generated migration for {schema_id}.{table_id}\n"
    ));
    b.push_str(&format!("-- Added columns: {}\n", added.len()));
    b.push_str("-- Review this migration before running.\n\n");
    for col in added {
        validate_path_component(&col.name)?;
        validate_type_literal(&col.type_name)?;
        let col_id = bracket_ident(&col.name)?;
        let null = if col.nullable { "NULL" } else { "NOT NULL" };
        b.push_str(&format!(
            "ALTER TABLE {schema_id}.{table_id} ADD {col_id} {typ} {null};\n",
            typ = col.type_name,
        ));
    }
    Ok(b)
}

fn validate_type_literal(typ: &str) -> Result<()> {
    if typ.is_empty() || typ.len() > 128 {
        return Err(crate::error::Error::InvalidInput(format!(
            "invalid type literal length: {typ:?}"
        )));
    }
    if !typ
        .chars()
        .all(|c| c.is_ascii() && !matches!(c, ';' | '\n' | '\r' | '[' | ']' | '\'' | '"'))
    {
        return Err(crate::error::Error::InvalidInput(format!(
            "invalid type literal: {typ:?}"
        )));
    }
    Ok(())
}
