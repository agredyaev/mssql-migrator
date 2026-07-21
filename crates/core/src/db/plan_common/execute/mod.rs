mod ctx;
mod dispatch;
mod setup;

use crate::cache::l1::L1Cache;
use crate::config::Config;
use crate::db::plan_db_trace::PlanDbTrace;
use crate::db::plan_snapshot::PlanDbResult;
use crate::domain::Workspace;
use crate::driver::TimingConn;
use crate::error::Result;

use dispatch::run_plan_body;
use setup::prepare_execute;

/// Cache-bypass and checksum-repair flags for the DB phase.
pub struct ExecOpts {
    pub bypass: bool,
    pub allow_checksum_repair: bool,
}

pub async fn execute(
    cfg: &Config,
    conn: &mut TimingConn,
    ws: &Workspace,
    keys_json: &str,
    fp: &str,
    l1: &L1Cache,
    opts: ExecOpts,
) -> Result<PlanDbResult> {
    let mut trace = PlanDbTrace::default();
    let setup = prepare_execute(
        cfg,
        conn,
        ws,
        keys_json,
        &mut trace,
        opts.bypass,
        opts.allow_checksum_repair,
    )
    .await?;
    let (ensure_ms, body, parallel_wall) =
        run_plan_body(cfg, conn, ws, keys_json, &setup, &mut trace).await?;
    let catalog = body.catalog;

    let io = conn.io_snapshot();
    trace.timings.query_calls = io.query_calls;
    trace.timings.query_ms = io.query_ms;

    if cfg.catalog_cache && !catalog.objects.is_empty() && (setup.need_catalog || setup.git_delta) {
        if let Err(e) = crate::db::save_batched(conn, &ws.layout_digest, ws, &catalog).await {
            tracing::warn!(error = %e, "catalog cache save failed");
        }
    }

    l1.save(fp, &ws.layout_digest, &body.checksums, &catalog)?;

    Ok(PlanDbResult {
        checksums: body.checksums,
        catalog,
        ensure_ms,
        checksums_ms: body.checksums_ms,
        inspect_ms: body.inspect_ms,
        parallel_wall_ms: parallel_wall,
        l1_hit: false,
        trace,
    })
}
