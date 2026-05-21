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
    crate::plan::rebuild_path_caches(ws);
    if !skip_git {
        crate::domain::intern_script_git_strings(ws);
    }
    ws.layout_digest = digest::layout_digest(ws);
    Ok(timings::dur_ms(t0.elapsed()))
}
