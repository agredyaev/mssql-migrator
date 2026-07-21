use crate::cache::l1::L1Cache;
use crate::config::Config;
use crate::db::plan_db_trace::{PlanDbPath, PlanDbTrace};
use crate::db::state::ChecksumMap;
use crate::domain::Workspace;
use crate::driver::TimingConn;
use crate::error::Result;

use super::plan_common::{execute, ExecOpts};

/// Output of the plan DB phase containing catalog state, checksums, and timing data.
pub struct PlanDbResult {
    /// Object checksum map loaded from audit history.
    pub checksums: ChecksumMap,
    /// Live catalog state for all objects in the workspace.
    pub catalog: crate::db::state::CatalogState,
    /// Milliseconds spent ensuring audit tables exist.
    pub ensure_ms: i64,
    /// Milliseconds spent loading checksum history.
    pub checksums_ms: i64,
    /// Milliseconds spent inspecting catalog object definitions.
    pub inspect_ms: i64,
    /// Wall-clock milliseconds for the parallel catalog inspect phase.
    pub parallel_wall_ms: i64,
    /// True when results were served from the L1 filesystem cache.
    pub l1_hit: bool,
    /// Structured trace of the DB phase execution path.
    pub trace: PlanDbTrace,
}

/// Loads catalog state and checksums, serving from L1 cache or warm snapshot when
/// available. When `bypass_cache` is set (mutating commands running under the
/// advisory lock), the local caches are skipped so the plan always reflects live
/// DB state — a restore/external change since the cache was written must not let
/// migrate believe the database is already at the target.
pub async fn run_plan_db_phase(
    cfg: &Config,
    conn: &mut TimingConn,
    ws: &Workspace,
    bypass_cache: bool,
    allow_checksum_repair: bool,
) -> Result<PlanDbResult> {
    let fp = crate::audit::db_fingerprint(&cfg.server, &cfg.port, &cfg.user, &cfg.database);
    let l1 = L1Cache::new(&cfg.l1_cache_dir);
    // Every managed object has a live-state fingerprint. A top-level snapshot
    // cannot prove that SQL Server was not changed out of band, so only an
    // empty workspace may use it.
    let requires_live_state = ws.object_count() != 0;

    if !bypass_cache && !requires_live_state {
        if let Some((checksums, catalog)) = l1.try_load(&fp, &ws.layout_digest)? {
            return Ok(empty_result(checksums, catalog, true, PlanDbPath::CacheHit));
        }
    }

    if let Some((checksums, catalog)) = super::warm_snapshot::reuse(&fp, &ws.layout_digest)
        .filter(|_| !bypass_cache && !requires_live_state)
    {
        l1.save(&fp, &ws.layout_digest, &checksums, &catalog)?;
        return Ok(empty_result(
            checksums,
            catalog,
            false,
            PlanDbPath::WarmSnapshot,
        ));
    }

    let keys_json = ws.normalized_keys_json();

    execute(
        cfg,
        conn,
        ws,
        &keys_json,
        &fp,
        &l1,
        ExecOpts {
            bypass: bypass_cache,
            allow_checksum_repair,
        },
    )
    .await
}

/// Cache/snapshot hit result: catalog state served with zero DB-phase timings.
fn empty_result(
    checksums: ChecksumMap,
    catalog: crate::db::state::CatalogState,
    l1_hit: bool,
    path: PlanDbPath,
) -> PlanDbResult {
    PlanDbResult {
        checksums,
        catalog,
        ensure_ms: 0,
        checksums_ms: 0,
        inspect_ms: 0,
        parallel_wall_ms: 0,
        l1_hit,
        trace: PlanDbTrace {
            path: Some(path),
            ..PlanDbTrace::default()
        },
    }
}
