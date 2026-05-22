use crate::cache::l1::L1Cache;
use crate::config::Config;
use crate::db::plan_db_trace::{PlanDbPath, PlanDbTrace};
use crate::db::state::ChecksumMap;
use crate::domain::Workspace;
use crate::driver::TimingConn;
use crate::error::Result;

use super::plan_batch::run_batch;
use super::plan_parallel::run_parallel;

pub struct PlanDbResult {
    pub checksums: ChecksumMap,
    pub catalog: crate::db::state::CatalogState,
    pub ensure_ms: i64,
    pub checksums_ms: i64,
    pub inspect_ms: i64,
    pub parallel_wall_ms: i64,
    pub l1_hit: bool,
    pub trace: PlanDbTrace,
}

pub async fn run_plan_db_phase(
    cfg: &Config,
    conn: &mut TimingConn,
    ws: &Workspace,
) -> Result<PlanDbResult> {
    let fp = format!("{}_{}", cfg.server, cfg.database);
    let l1 = L1Cache::new(&cfg.l1_cache_dir);

    if let Some((checksums, catalog)) = l1.try_load(&fp, &ws.layout_digest)? {
        return Ok(PlanDbResult {
            checksums,
            catalog,
            ensure_ms: 0,
            checksums_ms: 0,
            inspect_ms: 0,
            parallel_wall_ms: 0,
            l1_hit: true,
            trace: PlanDbTrace {
                path: Some(PlanDbPath::CacheHit),
                ..PlanDbTrace::default()
            },
        });
    }

    if let Some((checksums, catalog)) = super::warm_snapshot::reuse(&ws.layout_digest) {
        l1.save(&fp, &ws.layout_digest, &checksums, &catalog)?;
        return Ok(PlanDbResult {
            checksums,
            catalog,
            ensure_ms: 0,
            checksums_ms: 0,
            inspect_ms: 0,
            parallel_wall_ms: 0,
            l1_hit: false,
            trace: PlanDbTrace {
                path: Some(PlanDbPath::WarmSnapshot),
                ..PlanDbTrace::default()
            },
        });
    }

    let keys_json = ws.normalized_keys_json();

    if cfg.session_socket.is_empty() {
        run_parallel(cfg, conn, ws, &keys_json, &fp, &l1).await
    } else {
        run_batch(cfg, conn, ws, &keys_json, &fp, &l1).await
    }
}
