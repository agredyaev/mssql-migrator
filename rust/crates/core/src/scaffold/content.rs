use crate::db::TableColumn;

use super::dir::SCAFFOLD_MARK;

pub fn scaffold_content(schema: &str, table: &str, columns: &[TableColumn]) -> String {
    let mut b = String::new();
    b.push_str(SCAFFOLD_MARK);
    b.push('\n');
    b.push_str(&format!("-- Table: [{schema}].[{table}]\n"));
    b.push_str("-- Replace this scaffold with the actual migration SQL.\n");
    b.push_str(&format!("-- Schema: {schema}\n"));
    b.push_str(&format!("-- Table: {table}\n"));
    b.push_str("-- Press Ctrl+C to stop migration.\n");
    if !columns.is_empty() {
        b.push_str("-- Detected columns:\n");
        for col in columns {
            let null = if col.nullable { " NULL" } else { " NOT NULL" };
            b.push_str(&format!("--   {} {}{}\n", col.name, col.type_name, null));
        }
    }
    b
}
