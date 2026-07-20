use crate::db::state::ChecksumMap;

#[derive(Debug, Default)]
pub(crate) struct DiffCounters {
    pub create: i64,
    pub adopt: i64,
    pub skip: i64,
    pub changed: i64,
    pub blocked: i64,
}

pub(crate) struct DecideCtx<'a> {
    pub checksums: &'a ChecksumMap,
    pub counters: &'a mut DiffCounters,
}
