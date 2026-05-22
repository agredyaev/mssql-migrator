pub const PHASE_FLAG_L1_CACHE_HIT: u8 = 1 << 0;
pub const PHASE_FLAG_PLAN_DB_BOOTSTRAP: u8 = 1 << 1;
pub const PHASE_FLAG_PLAN_DB_CATALOG_QUERIED: u8 = 1 << 2;
pub const PHASE_FLAG_PLAN_DB_HISTORY_EMPTY: u8 = 1 << 3;
pub const PHASE_FLAG_PLAN_DB_CHECKSUMS_SKIPPED: u8 = 1 << 4;

#[inline]
pub(super) fn flag_get(flags: u8, mask: u8) -> bool {
    flags & mask != 0
}

#[inline]
pub(super) fn flag_set(flags: &mut u8, mask: u8, on: bool) {
    if on {
        *flags |= mask;
    } else {
        *flags &= !mask;
    }
}
