use crate::cache::l1::L1Cache;
use crate::config::Config;
use crate::db::plan_db_trace::{PlanDbPath, PlanDbTrace};
use crate::db::state::ChecksumMap;
use crate::domain::is_module_kind_code;
use crate::domain::Workspace;
use crate::driver::TimingConn;
use crate::error::Result;

use super::plan_batch::run_batch;
use super::plan_common::{ExecOpts, PlanDbMode};
use super::plan_parallel::run_parallel;

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
    // Module definitions are compared to live SQL Server text. A cached
    // checksum/catalog pair has no current definition digest, so never let a
    // top-level snapshot suppress that query.
    let has_modules = (0..ws.object_count()).any(|i| is_module_kind_code(ws.row(i).kind_code));

    if !bypass_cache && !has_modules {
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
    }

    if let Some((checksums, catalog)) = super::warm_snapshot::reuse(&fp, &ws.layout_digest)
        .filter(|_| !bypass_cache && !has_modules)
    {
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
        let opts = ExecOpts {
            mode: PlanDbMode::Parallel,
            bypass: bypass_cache,
            allow_checksum_repair,
        };
        run_parallel(cfg, conn, ws, &keys_json, &fp, &l1, opts).await
    } else {
        let opts = ExecOpts {
            mode: PlanDbMode::Sequential,
            bypass: bypass_cache,
            allow_checksum_repair,
        };
        run_batch(cfg, conn, ws, &keys_json, &fp, &l1, opts).await
    }
}
