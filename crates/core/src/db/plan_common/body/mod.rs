mod git_delta;
mod standard;

use crate::driver::TimingConn;
use crate::error::Result;

use super::types::{BodyOutput, RunBodyContext};

pub(super) async fn run_body(
    mut ctx: RunBodyContext<'_>,
    conn: &mut TimingConn,
) -> Result<BodyOutput> {
    if ctx.git_delta {
        git_delta::run_git_delta_body(&mut ctx, conn).await
    } else {
        standard::run_standard_body(&mut ctx, conn).await
    }
}
