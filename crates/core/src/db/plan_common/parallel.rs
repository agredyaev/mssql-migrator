use std::sync::Arc;
use std::time::Instant;

use crate::audit;
use crate::db::plan_db_trace::PlanDbTrace;
use crate::driver::TimingConn;
use crate::error::Result;
use crate::timings;

use super::body;
use super::conn::{PlanDbConn, SharedConn};
use super::trace::merge_trace;
use super::types::{BodyOutput, RunBodyContext, RunParallelContext};

pub(super) async fn run_parallel_with_ensure(
    conn: &mut TimingConn,
    ctx: RunParallelContext<'_>,
    trace: &mut PlanDbTrace,
) -> Result<(i64, BodyOutput)> {
    let client = conn.take_client()?;
    // The direct path keeps one TDS session and serialises access through an
    // async mutex while preserving a single owner for restoration below.
    let shared = Arc::new(tokio::sync::Mutex::new(client));
    let io = Arc::clone(&conn.io);
    let db_fp2 = ctx.db_fp.to_string();
    let keys = ctx.keys_json.to_string();
    let git2 = ctx.git.clone();
    let cfg = ctx.cfg.clone();
    let round_trips_start = trace.timings.round_trips;
    let ws = ctx.ws;
    let full = ctx.full;
    let git_delta = ctx.git_delta;
    let need_checksums = ctx.need_checksums;
    let need_catalog = ctx.need_catalog;
    let catalog_base = ctx.catalog_base;
    let bypass = ctx.bypass;

    let command_timeout = ctx.cfg.command_timeout;
    let ensure_fut = {
        let shared = shared.clone();
        let db_fp_ensure = db_fp2.clone();
        async move {
            let t0 = Instant::now();
            let mut c = shared.lock().await;
            super::conn::bounded(
                command_timeout,
                "ensure tables",
                audit::ensure_tables_on(&mut c, &db_fp_ensure),
            )
            .await?;
            Ok::<_, crate::error::Error>(timings::dur_ms(t0.elapsed()))
        }
    };

    let shared_body = Arc::clone(&shared);
    let body_fut = async {
        wait_tables_ensured(&db_fp2).await;
        let shared_conn = SharedConn {
            client: shared_body,
            io: io.clone(),
            timeout: command_timeout,
        };
        let mut db_conn = PlanDbConn::Shared(&shared_conn);
        body::run_body(
            RunBodyContext {
                cfg: &cfg,
                ws,
                keys_json: &keys,
                db_fp: &db_fp2,
                git: &git2,
                full,
                git_delta,
                need_checksums,
                need_catalog,
                catalog_base,
                round_trips_start,
                bootstrap_in_sql: false,
                bypass,
            },
            &mut db_conn,
        )
        .await
    };

    let (body_res, ensure_res) = tokio::join!(body_fut, ensure_fut);

    // Reclaim and restore the client BEFORE surfacing any error, so a plan-phase
    // failure never leaves the outer TimingConn empty (which would make the
    // subsequent advisory-lock release fail with "temporarily unavailable").
    let client = Arc::try_unwrap(shared)
        .map_err(|_| crate::error::Error::Sql("plan parallel: shared conn still in use".into()))?
        .into_inner();
    conn.restore_client(client)?;

    let ensure_ms = ensure_res?;
    let body = body_res?;
    merge_trace(trace, &body.trace);

    Ok((ensure_ms, body))
}

async fn wait_tables_ensured(db_fp: &str) {
    for _ in 0..10_000 {
        if audit::tables_ensured(db_fp) {
            return;
        }
        tokio::task::yield_now().await;
    }
}
