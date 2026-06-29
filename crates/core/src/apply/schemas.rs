use crate::domain::SchemaAction;
use crate::driver::TimingConn;
use crate::error::Result;
use crate::export::MigrationPlan;

use super::result::ApplyResult;

pub async fn apply_schemas(
    conn: &mut TimingConn,
    plan: &MigrationPlan,
    result: &mut ApplyResult,
) -> Result<()> {
    for s in &plan.schemas {
        if s.action != SchemaAction::CreateSchema {
            result.skipped += 1;
            continue;
        }
        let q = match build_create_schema_sql(&s.schema_name) {
            Ok(q) => q,
            Err(e) => {
                result.push_error(format!("invalid schema name {}: {e}", s.schema_name));
                return Ok(());
            }
        };
        // Mirror objects/transitions: a SQL failure is recorded and routed to
        // `finish()` (cache invalidation + joined error) instead of bubbling via
        // `?`, which would skip `finish()` and leave a partial apply uncleaned.
        if let Err(e) = conn.exec(&q).await {
            result.push_error(format!("schema {}: {e}", s.schema_name));
            return Ok(());
        }
        result.applied += 1;
    }
    Ok(())
}

/// Build an idempotent `CREATE SCHEMA` statement.
///
/// `CREATE SCHEMA` must be the first statement in its batch, so the existence
/// guard wraps it in `EXEC(...)` exactly as `sql/audit/bootstrap_tables.sql` does.
/// The name is validated and bracket-quoted via [`crate::sql_ident::bracket_ident`];
/// single quotes are doubled for both literal contexts (the `SCHEMA_ID` probe and
/// the `EXEC` string argument) so a name like `O'Brien` stays injection-safe.
fn build_create_schema_sql(name: &str) -> Result<String> {
    let bracket = crate::sql_ident::bracket_ident(name)?;
    let probe = name.replace('\'', "''");
    let exec_arg = format!("CREATE SCHEMA {bracket}").replace('\'', "''");
    Ok(format!(
        "IF SCHEMA_ID(N'{probe}') IS NULL EXEC('{exec_arg}')"
    ))
}

#[cfg(test)]
#[path = "../tests/schema_sql_test.rs"]
mod schema_sql_tests;
