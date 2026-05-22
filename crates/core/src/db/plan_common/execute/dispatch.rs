use std::time::Instant;

use crate::audit;
use crate::config::Config;
use crate::db::plan_db_trace::PlanDbTrace;
use crate::domain::Workspace;
use crate::driver::TimingConn;
use crate::error::Result;
use crate::timings;

use super::super::body;
use super::super::conn::PlanDbConn;
use super::super::parallel::run_parallel_with_ensure;
use super::super::trace::merge_trace;
use super::super::types::{BodyOutput, PlanDbMode};
use super::ctx::{body_ctx, parallel_ctx};
use super::setup::ExecuteSetup;

pub(super) async fn run_plan_body(
    cfg: &Config,
    conn: &mut TimingConn,
    ws: &Workspace,
    keys_json: &str,
    setup: &ExecuteSetup,
    mode: PlanDbMode,
    trace: &mut PlanDbTrace,
) -> Result<(i64, BodyOutput, i64)> {
    let t_par = Instant::now();
    let (ensure_ms, body) = match mode {
        PlanDbMode::Parallel if setup.need_bootstrap => {
            run_parallel_with_ensure(conn, parallel_ctx(cfg, ws, keys_json, setup), trace).await?
        }
        PlanDbMode::Sequential => {
            run_sequential_body(cfg, conn, ws, keys_json, setup, trace).await?
        }
        PlanDbMode::Parallel => run_parallel_body(cfg, conn, ws, keys_json, setup, trace).await?,
    };
    Ok((ensure_ms, body, timings::dur_ms(t_par.elapsed())))
}

async fn run_sequential_body(
    cfg: &Config,
    conn: &mut TimingConn,
    ws: &Workspace,
    keys_json: &str,
    setup: &ExecuteSetup,
    trace: &mut PlanDbTrace,
) -> Result<(i64, BodyOutput)> {
    let ems = if setup.need_bootstrap && !setup.defer_bootstrap {
        let t0 = Instant::now();
        audit::ensure_tables(conn, &setup.db_fp).await?;
        timings::dur_ms(t0.elapsed())
    } else {
        0
    };
    let mut db_conn = PlanDbConn::Timing(conn);
    let body = body::run_body(
        body_ctx(cfg, ws, keys_json, setup, trace, setup.bootstrap_in_sql),
        &mut db_conn,
    )
    .await?;
    merge_trace(trace, &body.trace);
    Ok((ems + body.ensure_ms, body))
}

async fn run_parallel_body(
    cfg: &Config,
    conn: &mut TimingConn,
    ws: &Workspace,
    keys_json: &str,
    setup: &ExecuteSetup,
    trace: &mut PlanDbTrace,
) -> Result<(i64, BodyOutput)> {
    let mut db_conn = PlanDbConn::Timing(conn);
    let body = body::run_body(
        body_ctx(cfg, ws, keys_json, setup, trace, false),
        &mut db_conn,
    )
    .await?;
    merge_trace(trace, &body.trace);
    Ok((0, body))
}
