use crate::config::Config;
use crate::db::plan_db_trace::PlanDbTrace;
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
    /// Structured trace of the DB phase execution path.
    pub trace: PlanDbTrace,
}

/// Loads live catalog state and checksums from SQL Server.
pub async fn run_plan_db_phase(
    cfg: &Config,
    conn: &mut TimingConn,
    ws: &Workspace,
    bypass_cache: bool,
    allow_checksum_repair: bool,
) -> Result<PlanDbResult> {
    let keys_json = ws.normalized_keys_json();

    execute(
        cfg,
        conn,
        ws,
        &keys_json,
        ExecOpts {
            bypass: bypass_cache,
            allow_checksum_repair,
        },
    )
    .await
}
