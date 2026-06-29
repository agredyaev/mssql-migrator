use crate::config::Config;
use crate::db::{load_table_columns, TableColumn};
use crate::domain::Workspace;
use crate::driver::TimingConn;
use crate::error::{Error, Result};
use crate::export::MigrationPlan;
use crate::scaffold;

pub async fn handle_blocked_migrate(
    conn: &mut TimingConn,
    cfg: &Config,
    ws: &Workspace,
    plan: &MigrationPlan,
) -> Result<()> {
    let columns: std::collections::HashMap<String, Vec<TableColumn>> =
        load_table_columns(conn, ws).await?;
    scaffold::ensure(cfg, ws, plan, &columns)?;
    Err(Error::PlanBlocked)
}
