use crate::cache::l1::L1Cache;
use crate::config::Config;
use crate::domain::Workspace;
use crate::driver::TimingConn;
use crate::error::Result;

use super::plan_common::{execute, ExecOpts};
use super::plan_snapshot::PlanDbResult;

pub async fn run_parallel(
    cfg: &Config,
    conn: &mut TimingConn,
    ws: &Workspace,
    keys_json: &str,
    fp: &str,
    l1: &L1Cache,
    opts: ExecOpts,
) -> Result<PlanDbResult> {
    execute(cfg, conn, ws, keys_json, fp, l1, opts).await
}
