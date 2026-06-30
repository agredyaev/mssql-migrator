/// Phase flag: L1 disk cache was hit, skipping the catalog query round-trip.
pub const PHASE_FLAG_L1_CACHE_HIT: u8 = 1 << 0;
/// Phase flag: plan-DB bootstrap step ran to create audit tables.
pub const PHASE_FLAG_PLAN_DB_BOOTSTRAP: u8 = 1 << 1;
/// Phase flag: full catalog was queried from SQL Server.
pub const PHASE_FLAG_PLAN_DB_CATALOG_QUERIED: u8 = 1 << 2;
/// Phase flag: audit history table was found to be empty; baseline not yet run.
pub const PHASE_FLAG_PLAN_DB_HISTORY_EMPTY: u8 = 1 << 3;
/// Phase flag: checksum load was skipped (history empty or L1 cache covered it).
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
