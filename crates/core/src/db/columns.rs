use std::collections::HashMap;

use serde_json::json;

use super::state::TableColumn;
use crate::domain::Workspace;
use crate::driver::{RowData, TimingConn};
use crate::error::Result;
use crate::sql;

/// Loads column metadata for all tables in `ws` from the catalog.
pub async fn load_table_columns(
    conn: &mut TimingConn,
    ws: &Workspace,
) -> Result<HashMap<String, Vec<TableColumn>>> {
    let refs: Vec<_> = ws
        .object_entries
        .iter()
        .enumerate()
        .filter(|(i, o)| o.kind_part(ws, *i) == "tables")
        .map(|(i, o)| {
            json!({
                "schema": o.schema_part(ws, i).to_lowercase(),
                "kind": "tables",
                "object": o.name_part(ws, i).to_lowercase(),
            })
        })
        .collect();
    if refs.is_empty() {
        return Ok(HashMap::new());
    }
    let arg = serde_json::to_string(&refs).unwrap_or_else(|_| "[]".into());
    let rows = conn
        .query(sql::catalog::COLUMNS_OPENJSON, &[arg.as_str()])
        .await?;
    Ok(rows_into_map(rows))
}

fn rows_into_map(rows: Vec<RowData>) -> HashMap<String, Vec<TableColumn>> {
    let mut out: HashMap<String, Vec<TableColumn>> = HashMap::new();
    for row in rows {
        let schema = row.get_str(0).unwrap_or("").to_lowercase();
        let table = row.get_str(1).unwrap_or("").to_lowercase();
        let name = row.get_str(2).unwrap_or("").to_lowercase();
        let type_name = row.get_str(3).unwrap_or("").to_lowercase();
        let nullable = row_nullable(&row, 7);
        let key = format!("{schema}/tables/{table}");
        out.entry(key).or_default().push(TableColumn {
            name,
            type_name,
            nullable,
        });
    }
    out
}

fn row_nullable(row: &RowData, idx: usize) -> bool {
    if let Some(s) = row.get_str(idx) {
        return matches!(s, "1" | "true");
    }
    if let Some(b) = row.get_bytes(idx) {
        return !b.is_empty() && b[0] != 0;
    }
    true
}
