//! SQL schema filesystem tree scans, filename parsing, and MD5 digests.
//!
//! ### Purpose
//! Scans the configured filesystem schema layouts, parsing table directories, views, procedures,
//! and migration scripts to build a strongly-typed in-memory representation (`Workspace`).
//!
//! ### Architectural Context
//! - **Inputs**: Local file paths, SQL script bytes.
//! - **Outputs**: Hydrated `Workspace` entities containing layout objects and schema digests.
//! - **Boundaries**: Operates strictly on the configured schema directory tree, avoiding out-of-bounds files.
//!
//! ### Nominal Flow
//! 1. Walk directory tree to identify structural SQL schemas (`scan_root`).
//! 2. Parse structural parameters and git commit logs for individual scripts.
//! 3. Build string caches and rebuild path lookups inside the workspace (`populate`).
//! 4. Compute unique schema MD5 layout hashes to assert validation parity (`layout_digest`).
//!
//! ### Off-Nominal & Failure Containment
//! - **Malformed Schema Layout / Parse Failure**: Returns `Error::InvalidInput` immediately, preventing invalid schema trees from being processed by planning.

mod digest;
pub use digest::layout_digest;
mod git_log;
mod git_preload;
mod git_repo;
mod parse;
mod parse_object;
pub mod transition;
mod walk;

pub use walk::scan_root;

use std::time::Instant;

use crate::domain::Workspace;
use crate::error::Result;
use crate::timings;

pub async fn populate(ws: &mut Workspace, root: &str, skip_git: bool) -> Result<i64> {
    let t0 = Instant::now();
    walk::scan_root(ws, root)?;
    if !skip_git {
        git_preload::preload(ws, root);
    }
    crate::domain::intern_workspace_strings(ws);
    crate::domain::rebuild_path_caches(ws);
    if !skip_git {
        crate::domain::intern_script_git_strings(ws);
    }
    ws.layout_digest = digest::layout_digest(ws);
    Ok(timings::dur_ms(t0.elapsed()))
}
