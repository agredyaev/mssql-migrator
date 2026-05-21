use crate::db::state::ChecksumMap;
use crate::domain::Workspace;
use crate::export::MigrationPlan;

/// Mirror `Workspace::prior_by_row` into the plan (built in [`crate::plan::scope::apply_checksums_if_needed`]).
pub fn fill_prior_by_row(plan: &mut MigrationPlan, ws: &Workspace, _checksums: &ChecksumMap) {
    if ws.checksums_applied() {
        plan.prior_by_row.clone_from(&ws.prior_by_row);
    }
}

pub(crate) fn prior_digest_present(priors: &[Option<[u8; 32]>], row_index: usize) -> bool {
    priors
        .get(row_index)
        .and_then(|o| *o)
        .is_some_and(|cs| cs != [0; 32])
}
