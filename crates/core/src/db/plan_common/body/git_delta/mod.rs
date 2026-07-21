mod catalog;
mod query;
mod warmup;

use crate::driver::TimingConn;
use crate::error::Result;

use super::super::types::{BodyOutput, RunBodyContext};

use catalog::query_git_delta_catalog;
use warmup::warmup_git_delta;

pub(super) async fn run_git_delta_body(
    ctx: &mut RunBodyContext<'_>,
    conn: &mut TimingConn,
) -> Result<BodyOutput> {
    let warm = warmup_git_delta(ctx, conn).await?;
    let (loaded, inspect_ms, warm) = query_git_delta_catalog(ctx, conn, warm).await?;

    Ok(BodyOutput {
        checksums: warm.checksums,
        catalog: loaded,
        checksums_ms: warm.checksums_ms,
        inspect_ms,
        ensure_ms: 0,
        trace: warm.local_trace,
        _round_trips: warm.round_trips,
    })
}
