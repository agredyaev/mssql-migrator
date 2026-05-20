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
        let q = match crate::sql_ident::bracket_ident(&s.schema_name) {
            Ok(ident) => format!("CREATE SCHEMA {ident}"),
            Err(e) => {
                result.push_error(format!("invalid schema name {}: {e}", s.schema_name));
                return Ok(());
            }
        };
        conn.exec(&q).await?;
        result.applied += 1;
    }
    Ok(())
}
