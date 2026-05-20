//! DDL / scaffold probes for git workflow tests.

use migrator_core::driver::TimingConn;
use migrator_core::error::Result;

pub async fn table_column_exists(
    conn: &mut TimingConn,
    schema: &str,
    table: &str,
    column: &str,
) -> Result<bool> {
    let rows = conn
        .query(
            r"SELECT COUNT(*)
FROM sys.columns c
INNER JOIN sys.tables t ON t.object_id = c.object_id
INNER JOIN sys.schemas s ON s.schema_id = t.schema_id
WHERE s.name = @p1 AND t.name = @p2 AND c.name = @p3",
            &[schema, table, column],
        )
        .await?;
    Ok(rows.first().and_then(|r| r.get_i32(0)).unwrap_or(0) > 0)
}

pub fn read_scaffold_sql(sql_root: &std::path::Path) -> Option<String> {
    let dir = sql_root.join("dactests/smoke/tables/_migrations/smoke_table");
    let entries = std::fs::read_dir(&dir).ok()?;
    for e in entries.flatten() {
        let p = e.path();
        if p.is_file() && p.extension().is_some_and(|x| x == "sql") {
            return std::fs::read_to_string(p).ok();
        }
    }
    None
}
