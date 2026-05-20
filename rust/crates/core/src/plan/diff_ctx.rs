use crate::db::state::{CatalogState, ChecksumMap};
use crate::export::MigrationPlan;

#[derive(Debug, Default)]
pub(crate) struct DiffCounters {
    pub create: i64,
    pub adopt: i64,
    pub skip: i64,
    pub changed: i64,
    pub blocked: i64,
}

pub(crate) struct DecideCtx<'a> {
    pub catalog: &'a CatalogState,
    pub checksums: &'a ChecksumMap,
    pub plan: &'a mut MigrationPlan,
    pub counters: &'a mut DiffCounters,
}
