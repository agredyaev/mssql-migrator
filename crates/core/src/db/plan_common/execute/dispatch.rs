use std::time::{Duration, Instant};

use crate::audit;
use crate::config::Config;
use crate::db::plan_db_trace::PlanDbTrace;
use crate::domain::Workspace;
use crate::driver::TimingConn;
use crate::error::{Error, Result};
use crate::timings;

use super::super::body;
use super::super::trace::merge_trace;
use super::super::types::BodyOutput;
use super::ctx::body_ctx;
use super::setup::ExecuteSetup;

/// Bound `fut` by the command timeout `t`; `Duration::ZERO` disables it.
async fn bounded<T>(
    t: Duration,
    what: &str,
    fut: impl std::future::Future<Output = Result<T>>,
) -> Result<T> {
    if t.is_zero() {
        return fut.await;
    }
    match tokio::time::timeout(t, fut).await {
        Ok(r) => r,
        Err(_) => Err(Error::Sql(format!("{what} timed out after {t:?}"))),
    }
}

pub(super) async fn run_plan_body(
    cfg: &Config,
    conn: &mut TimingConn,
    ws: &Workspace,
    keys_json: &str,
    setup: &ExecuteSetup,
    trace: &mut PlanDbTrace,
) -> Result<(i64, BodyOutput, i64)> {
    let t_par = Instant::now();
    let direct = cfg.session_socket.is_empty();
    // Non-daemon (direct) runs eager-ensure under the command timeout; daemon
    // runs may defer bootstrap into the body SQL batch.
    let defer = setup.defer_bootstrap && !direct;
    let ems = if setup.need_bootstrap && !defer {
        let t0 = Instant::now();
        if direct {
            bounded(
                cfg.command_timeout,
                "ensure tables",
                audit::ensure_tables(conn, &setup.db_fp),
            )
            .await?;
        } else {
            audit::ensure_tables(conn, &setup.db_fp).await?;
        }
        timings::dur_ms(t0.elapsed())
    } else {
        0
    };
    let body = body::run_body(body_ctx(cfg, ws, keys_json, setup, trace, defer), conn).await?;
    merge_trace(trace, &body.trace);
    Ok((ems + body.ensure_ms, body, timings::dur_ms(t_par.elapsed())))
}
